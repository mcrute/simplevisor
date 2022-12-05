package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
)

func StdoutWriter(ctx context.Context, wg *sync.WaitGroup, stdout io.Writer, logger *InternalLogger) {
	wg.Add(1)
	defer wg.Done()

	writer := json.NewEncoder(stdout)

	for {
		select {
		case r := <-logger.Logs:
			if err := writer.Encode(r); err != nil {
				logger.Logf("StdoutWriter: Error marshalling log: %s", err)
			}
			logger.Pool.Put(r)
		case <-ctx.Done():
			return
		}
	}
}

func ProcessLogHandler(ctx context.Context, wg *sync.WaitGroup, logger *InternalLogger, rawStream io.Reader, name string, streamType StreamType) {
	wg.Add(1)
	defer wg.Done()

	stream := bufio.NewScanner(rawStream)

	var msg *LogRecord
	var done bool

	for stream.Scan() {
		msg = logger.Pool.Get().FromNow().FromProcess(name).FromStream(streamType)
		msg.Message.Write(stream.Bytes())

		select {
		case logger.Logs <- msg:
			if done {
				return
			}
		case <-ctx.Done():
			return
		default:
		}
	}
}
