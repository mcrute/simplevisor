package supervise

import (
	"os"
	"os/signal"
	"syscall"
)

func SetupSignals() chan os.Signal {
	sigs := make(chan os.Signal, 10)
	signal.Notify(sigs)

	// Ignore these to prevent losing the TTY
	signal.Ignore(syscall.SIGTTIN, syscall.SIGTTOU)

	// These are for us and should not be proxied
	signal.Reset(syscall.SIGFPE, syscall.SIGILL, syscall.SIGSEGV, syscall.SIGBUS, syscall.SIGABRT, syscall.SIGTRAP, syscall.SIGSYS)

	return sigs
}
