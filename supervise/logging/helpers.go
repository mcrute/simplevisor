package logging

import (
	"fmt"
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
