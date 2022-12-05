package supervise

import (
	"syscall"
)

const exitSignalOffset = 128

type exit struct {
	Pid    int
	Status int
}

func ReapChildren() ([]exit, error) {
	var ws syscall.WaitStatus
	var rus syscall.Rusage
	var exits []exit

	for {
		pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, &rus)
		if err != nil {
			if err == syscall.ECHILD {
				return exits, nil
			}
			return exits, err
		}

		if pid <= 0 {
			return exits, nil
		}

		status := ws.ExitStatus()
		if ws.Signaled() {
			status = exitSignalOffset + int(ws.Signal())
		}

		exits = append(exits, exit{
			Pid:    pid,
			Status: status,
		})
	}
}
