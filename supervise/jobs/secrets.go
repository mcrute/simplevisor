package jobs

import (
	"context"
	"sync"

	"code.crute.us/mcrute/golib/secrets"
	"code.crute.us/mcrute/simplevisor/supervise/logging"
)

func SecretsLogger(ctx context.Context, wg *sync.WaitGroup, sc secrets.ClientManager, logger *logging.InternalLogger) {
	wg.Add(1)
	defer wg.Done()

	notifications := sc.Notifications()
	for {
		select {
		case n := <-notifications:
			if n.Critical && n.Error != nil {
				logger.Fatalf("Error in renewing secrets: %s", n.Error)
			} else {
				logger.Logf("Credential %s renewed at %s", n.Name, n.Time)
			}
		case <-ctx.Done():
			return
		}
	}
}
