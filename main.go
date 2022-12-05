package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"code.crute.us/mcrute/golib/secrets"
	"code.crute.us/mcrute/simplevisor/supervise"
	"code.crute.us/mcrute/simplevisor/supervise/jobs"
	"code.crute.us/mcrute/simplevisor/supervise/logging"
	"golang.org/x/sys/unix"
)

const (
	modeParent = "parent"
	modeChild  = "child"
)

func readConfig(path string) (*supervise.AppConfig, error) {
	cf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("readConfig: unable to load config")
	}

	cfg := &supervise.AppConfig{}
	if err := json.Unmarshal(cf, &cfg); err != nil {
		return nil, fmt.Errorf("readConfig: unable to parse config: %s", err)
	}

	return cfg, nil
}

func parentMain(cfgLoc string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := supervise.SetupSignals()

	wg := &sync.WaitGroup{}
	logger := &logging.InternalLogger{
		Logs:      make(chan *logging.LogRecord, 100),
		Pool:      logging.NewBufferPool(),
		Cancel:    cancel,
		WaitGroup: wg,
	}
	go logging.StdoutWriter(ctx, wg, os.Stdout, logger)

	cfg, err := readConfig(cfgLoc)
	if err != nil {
		logger.Fatalf("parentMain: error loading config: %s", err)
	}

	vc, err := secrets.NewVaultClient(&secrets.VaultClientConfig{})
	if err != nil {
		logger.Fatalf("parentMain: unable to setup vault: %s", err)
	}

	if err := vc.Authenticate(ctx); err != nil {
		logger.Fatalf("parentMain: unable to auth vault: %s", err)
	}

	// TODO: Support VAULT_TOKEN
	env, err := supervise.PrepareEnvironment(ctx, cfg.Environment, vc, "")
	if err != nil {
		logger.Fatalf("parentMain: unable to prepare environment: %s", err)
	}

	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, uintptr(1), 0, 0, 0); err != nil {
		logger.Fatalf("parentMain: unable to become subreaper: %s", err)
	}

	go jobs.SecretsLogger(ctx, wg, vc, logger)
	go vc.Run(ctx, wg)

	runner := &supervise.CommandRunner{
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
	handles := []*supervise.CommandHandle{}
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
		exits, err := supervise.ReapChildren()
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

				supervise.ReapChildren()
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

func main() {
	mode := flag.String("mode", modeParent, "mode in which to run simplevisor, internal use only")
	config := flag.String("config", "simplevisor.json", "config file location")
	flag.Parse()

	switch *mode {
	case modeParent:
		parentMain(*config)
	case modeChild:
		supervise.ChildMain()
	default:
		panic("TODO: Add error, invalid mode")
	}
}
