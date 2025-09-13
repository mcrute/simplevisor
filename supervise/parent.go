package supervise

import (
	"context"
	"os"
	"sync"
	"syscall"
	"time"

	"code.crute.us/mcrute/golib/secrets"
	"code.crute.us/mcrute/simplevisor/supervise/jobs"
	"code.crute.us/mcrute/simplevisor/supervise/logging"
	"golang.org/x/sys/unix"
)

type SupervisorParent struct {
	handles []*CommandHandle
	cancel  func()
	wg      *sync.WaitGroup
	log     *logging.InternalLogger
}

func (p *SupervisorParent) Main(cfgLoc string, disableVault bool, discoverVault bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.cancel = cancel
	p.handles = []*CommandHandle{}
	p.wg = &sync.WaitGroup{}

	sigs := SetupSignals()
	secretFailures := make(chan error)

	p.log = &logging.InternalLogger{
		Logs:      make(chan *logging.LogRecord, 100),
		Pool:      logging.NewBufferPool(),
		Cancel:    cancel,
		WaitGroup: p.wg,
	}
	go logging.StdoutWriter(ctx, p.wg, os.Stdout, p.log)

	cfg, err := ReadAppConfig(cfgLoc)
	if err != nil {
		p.fatal("parentMain: error loading config: %s", err)
		return
	}

	var vc secrets.ClientManager
	if !disableVault {
		if discoverVault {
			vc, err = secrets.NewAutodiscoverVaultClient(ctx)
		} else {
			vc, err = secrets.NewVaultClient(&secrets.VaultClientConfig{})
		}
		if err != nil {
			p.fatal("parentMain: unable to setup vault: %s", err)
			return
		}
	} else {
		vc, _ = secrets.NewNoopClient()
	}

	if err := vc.Authenticate(ctx); err != nil {
		p.fatal("parentMain: unable to auth vault: %s", err)
		return
	}

	// TODO: Support VAULT_TOKEN
	env, err := PrepareEnvironment(ctx, cfg.Environment, vc, "")
	if err != nil {
		p.fatal("parentMain: unable to prepare environment: %s", err)
		return
	}

	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, uintptr(1), 0, 0, 0); err != nil {
		p.fatal("parentMain: unable to become subreaper: %s", err)
		return
	}

	go jobs.SecretsLogger(ctx, p.wg, vc, p.log, secretFailures)
	go vc.Run(ctx, p.wg)

	runner := &CommandRunner{
		Logger:      p.log,
		BaseContext: ctx,
		WaitGroup:   p.wg,
		Environment: env,
	}

	if cfg.Jobs.Init != nil {
		for _, js := range cfg.Jobs.Init {
			p.log.Logf("parentMain: attempting to start job %s", js.Name)

			hnd, err := runner.Run(js)
			if err != nil {
				p.fatal("parentMain: error starting init job %s: %s", js.Name, err)
				return
			}
			if err := hnd.Wait(); err != nil {
				p.fatal("parentMain: error running init job %s: %s", js.Name, err)
				return
			}
			if exit := hnd.ExitCode(); exit != 0 {
				p.fatal("parentMain: error init job %s exited non-zero: %d", js.Name, exit)
				return
			}
			hnd.Cleanup()
		}
	}

	// TODO: Restart if crashed
	for _, js := range cfg.Jobs.Main {
		hnd, err := runner.Run(js)
		if err != nil {
			p.fatal("parentMain: error starting main job %s: %s", js.Name, err)
			return
		}
		p.handles = append(p.handles, hnd)
	}

	// TODO: Clean this up
	// Reap children and propogate signals until the end
	for {
		exits, err := ReapChildren()
		if err != nil {
			p.log.Logf("Error reaping children: %s", err)
		} else {
			for _, e := range exits {
				p.log.Logf("Reaped child %d with exit %d", e.Pid, e.Status)
			}
		}

		select {
		case s := <-sigs:
			switch s {
			case syscall.SIGTERM, syscall.SIGINT:
				p.Terminate(true)
				return
			}

			for _, h := range p.handles {
				h.Signal(s)
			}
		case f := <-secretFailures:
			p.log.Logf("%s", f)
			p.Terminate(false)
			return
		case <-ctx.Done():
			p.Terminate(true)
			return
		case <-time.After(time.Second):
		}
	}
}

func (p *SupervisorParent) fatal(msg string, args ...any) {
	p.log.Logf(msg, args...)
	p.Terminate(false)
}

func (p *SupervisorParent) Terminate(success bool) {
	for _, h := range p.handles {
		h.Terminate()
	}

	p.cancel()
	p.wg.Wait()

	ReapChildren()

	if success {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
