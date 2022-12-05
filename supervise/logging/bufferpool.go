package logging

import (
	"sync"
)

const bufferSize = 4000

type BufferPool struct {
	sync.Pool
}

func NewBufferPool() *BufferPool {
	return &BufferPool{
		sync.Pool{
			New: func() any {
				return (&LogRecord{}).Reset()
			},
		},
	}
}

func (p *BufferPool) Get() *LogRecord {
	return p.Pool.Get().(*LogRecord)
}

func (p *BufferPool) Put(b *LogRecord) {
	p.Pool.Put(b.Reset())
}
