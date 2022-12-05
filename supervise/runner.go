package supervise

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"code.crute.us/mcrute/simplevisor/supervise/logging"
)

type controlMessage struct {
	Command     []string
	Environment []string
	User        int
	Group       int
}

type CommandHandle struct {
	cmd            *exec.Cmd
	killsig        syscall.Signal
	stdout, stderr *os.File
	cancel         func()
}

func (h *CommandHandle) Cleanup() {
	h.cancel()
	h.stdout.Close()
	h.stderr.Close()
}

func (h *CommandHandle) Terminate() error {
	err := h.cmd.Process.Signal(h.killsig)
	h.Cleanup()
	return err
}

func (h *CommandHandle) Signal(sig os.Signal) error {
	return h.cmd.Process.Signal(sig)
}

func (h *CommandHandle) ExitCode() int {
	return h.cmd.ProcessState.ExitCode()
}

func (h *CommandHandle) Pid() int {
	return h.cmd.Process.Pid
}

func (h *CommandHandle) Wait() error {
	return h.cmd.Wait()
}

type CommandRunner struct {
	Logger      *logging.InternalLogger
	BaseContext context.Context
	WaitGroup   *sync.WaitGroup
	Environment []string
}

func (r *CommandRunner) Run(spec *Command) (*CommandHandle, error) {
	ctx, cancel := context.WithCancel(r.BaseContext)

	cmdR, cmdW := mustPipe()
	soR, soW := mustPipe()
	seR, seW := mustPipe()

	cmd := &exec.Cmd{
		Path:       os.Args[0],
		Args:       append([]string{os.Args[0]}, "--mode=child"),
		Stdin:      nil,
		Stdout:     soW,
		Stderr:     seW,
		ExtraFiles: []*os.File{cmdR},
	}

	go logging.ProcessLogHandler(ctx, r.WaitGroup, r.Logger, soR, spec.Name, logging.Stdout)
	go logging.ProcessLogHandler(ctx, r.WaitGroup, r.Logger, seR, spec.Name, logging.Stderr)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("Run: Error starting subprocess: %w", err)
	}

	hnd := &CommandHandle{
		cmd:     cmd,
		stdout:  soR,
		stderr:  seR,
		cancel:  cancel,
		killsig: spec.KillSignal,
	}

	// Cleanup child fds
	cmdR.Close()
	soW.Close()
	seW.Close()

	uid, err := getUid(spec.RunAsUser)
	if err != nil {
		hnd.Terminate()
		return nil, fmt.Errorf("Run: unable to resolve uid: %w", err)
	}

	gid, err := getGid(spec.RunAsGroup)
	if err != nil {
		hnd.Terminate()
		return nil, fmt.Errorf("Run: unable to resolve gid: %w", err)
	}

	if err := json.NewEncoder(cmdW).Encode(controlMessage{
		Command:     spec.Command,
		Environment: r.Environment,
		User:        uid,
		Group:       gid,
	}); err != nil {
		hnd.Terminate()
		return nil, fmt.Errorf("Run: Error writing to subprocess: %w", err)
	}
	cmdW.Close()

	return hnd, nil
}

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
