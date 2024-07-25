package supervise

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
)

func ChildMain() {
	fd := os.NewFile(uintptr(3), "commandPipe")
	if fd == nil {
		fmt.Println("childMain: error unable to open parent pipe")
		os.Exit(1)
	}

	cmd := &controlMessage{}
	if err := json.NewDecoder(fd).Decode(&cmd); err != nil {
		fmt.Printf("childMain: error decoding json: %s", err)
		os.Exit(1)
	}

	fd.Close()
	os.Stdin.Close()

	syscall.Setuid(cmd.User)
	syscall.Setgid(cmd.Group)

	// Start a session so signals are correctly delivered even to shell
	// subprocesses
	_, err := syscall.Setsid()
	if err != nil {
		fmt.Printf("childMain: error starting session: %s", err)
		os.Exit(1)
	}

	syscall.Exec(cmd.Command[0], cmd.Command, cmd.Environment)
}
