package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

const maxBufferSize = 4000 // 4kbytes

type StreamType int

const (
	Stdout StreamType = iota
	Stderr
)

type LogRecord struct {
	Process string
	Time    int64
	Stream  StreamType
	Message io.ReadWriter
}

func (r *LogRecord) FromProcess(p string) *LogRecord {
	r.Process = p
	return r
}

func (r *LogRecord) FromNow() *LogRecord {
	r.Time = time.Now().Unix()
	return r
}

func (r *LogRecord) FromStream(s StreamType) *LogRecord {
	r.Stream = s
	return r
}

func (r *LogRecord) WithMessage(s string) *LogRecord {
	r.Message.(*bytes.Buffer).WriteString(s)
	return r
}

func (r *LogRecord) Cap() int {
	return r.Message.(*bytes.Buffer).Cap()
}

func (r *LogRecord) Reset() *LogRecord {
	r.Process = ""
	r.Time = time.Now().Unix()
	r.Stream = Stdout

	if r.Message == nil {
		r.Message = &bytes.Buffer{}
	} else {
		// Free buffers that are too big
		if r.Message.(*bytes.Buffer).Cap() > maxBufferSize {
			r.Message = &bytes.Buffer{}
		}
		r.Message.(*bytes.Buffer).Reset()
	}

	return r
}

func (r *LogRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Process string     `json:"process"`
		Time    int64      `json:"time"`
		Stream  StreamType `json:"stream"`
		Message string     `json:"message"`
	}{
		r.Process,
		r.Time,
		r.Stream,
		string(r.Message.(*bytes.Buffer).Bytes()),
	})
}
