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

func ParentMain(cfgLoc string, disableVault bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := SetupSignals()

	wg := &sync.WaitGroup{}
	logger := &logging.InternalLogger{
		Logs:      make(chan *logging.LogRecord, 100),
		Pool:      logging.NewBufferPool(),
		Cancel:    cancel,
		WaitGroup: wg,
	}
	go logging.StdoutWriter(ctx, wg, os.Stdout, logger)

	cfg, err := ReadAppConfig(cfgLoc)
	if err != nil {
		logger.Fatalf("parentMain: error loading config: %s", err)
	}

	var vc secrets.ClientManager
	if !disableVault {
		vc, err = secrets.NewVaultClient(&secrets.VaultClientConfig{})
		if err != nil {
			logger.Fatalf("parentMain: unable to setup vault: %s", err)
		}
	} else {
		vc, _ = secrets.NewNoopClient()
	}

	if err := vc.Authenticate(ctx); err != nil {
		logger.Fatalf("parentMain: unable to auth vault: %s", err)
	}

	// TODO: Support VAULT_TOKEN
	env, err := PrepareEnvironment(ctx, cfg.Environment, vc, "")
	if err != nil {
		logger.Fatalf("parentMain: unable to prepare environment: %s", err)
	}

	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, uintptr(1), 0, 0, 0); err != nil {
		logger.Fatalf("parentMain: unable to become subreaper: %s", err)
	}

	go jobs.SecretsLogger(ctx, wg, vc, logger)
	go vc.Run(ctx, wg)

	runner := &CommandRunner{
		Logger:      logger,
		BaseContext: ctx,
		WaitGroup:   wg,
		Environment: env,
	}

	if cfg.Jobs.Init != nil {
		for _, js := range cfg.Jobs.Init {
			logger.Logf("parentMain: attempting to start job %s", js.Name)

			hnd, err := runner.Run(js)
			if err != nil {
				logger.Fatalf("parentMain: error starting init job %s: %s", js.Name, err)
			}
			if err := hnd.Wait(); err != nil {
				logger.Fatalf("parentMain: error running init job %s: %s", js.Name, err)
			}
			if exit := hnd.ExitCode(); exit != 0 {
				logger.Fatalf("parentMain: error init job %s exited non-zero: %d", js.Name, exit)
			}
			hnd.Cleanup()
		}
	}

	// TODO: Restart if crashed
	handles := []*CommandHandle{}
	for _, js := range cfg.Jobs.Main {
		hnd, err := runner.Run(js)
		if err != nil {
			logger.Fatalf("parentMain: error starting main job %s: %s", js.Name, err)
		}
		handles = append(handles, hnd)
	}

	// TODO: Clean this up
	// Reap children and propogate signals until the end
	for {
		exits, err := ReapChildren()
		if err != nil {
			logger.Logf("Error reaping children: %s", err)
		} else {
			for _, e := range exits {
				logger.Logf("Reaped child %d with exit %d", e.Pid, e.Status)
			}
		}

		select {
		case s := <-sigs:
			switch s {
			case syscall.SIGTERM, syscall.SIGINT:
				for _, h := range handles {
					h.Terminate()
				}

				cancel()
				wg.Wait()

				ReapChildren()
				return
			}

			for _, h := range handles {
				h.Signal(s)
			}
		case <-ctx.Done():
			break
		case <-time.After(time.Second):
		}
	}
}
