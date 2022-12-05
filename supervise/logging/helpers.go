package logging

import (
	"fmt"
	"os"
	"sync"
)

type InternalLogger struct {
	Logs      chan *LogRecord
	Pool      *BufferPool
	Cancel    func()
	WaitGroup *sync.WaitGroup
}

func (l *InternalLogger) Log(message string) {
	l.Logs <- l.Pool.Get().FromProcess("internal").FromNow().FromStream(Stdout).WithMessage(message)
}

func (l *InternalLogger) Logf(message string, args ...any) {
	l.Log(fmt.Sprintf(message, args...))
}

func (l *InternalLogger) Fatalf(message string, args ...any) {
	l.Fatal(fmt.Sprintf(message, args...))
}

func (l *InternalLogger) Fatal(message string) {
	l.Log(message)
	l.Cancel()
	l.WaitGroup.Wait()
	os.Exit(1)
}
