package jobs

import (
	"context"
	"fmt"
	"sync"

	"code.crute.us/mcrute/golib/secrets"
	"code.crute.us/mcrute/simplevisor/supervise/logging"
)

func SecretsLogger(ctx context.Context, wg *sync.WaitGroup, sc secrets.ClientManager, logger *logging.InternalLogger, failures chan error) {
	wg.Add(1)
	defer wg.Done()

	notifications := sc.Notifications()
	for {
		select {
		case n := <-notifications:
			if n.Critical && n.Error != nil {
				failures <- fmt.Errorf("Error in renewing secrets: %w", n.Error)
			} else {
				logger.Logf("Credential %s renewed at %s", n.Name, n.Time)
			}
		case <-ctx.Done():
			return
		}
	}
}
