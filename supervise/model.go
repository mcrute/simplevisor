package supervise

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"syscall"
)

//go:generate go run ../generate_syscall/main.go

type AppConfig struct {
	Environment *EnvConfig  `json:"env"`
	Jobs        *JobsConfig `json:"jobs"`
}

type JobsConfig struct {
	// Init jobs are a list of jobs that will be run serially before the
	// main jobs start. Any non-zero return code from these jobs will
	// result in the supervisor terminating startup.
	Init []*Command `json:"init"`

	// Main jobs are a list of jobs that are run in parallel after the
	// init jobs. These jobs should run in the foreground and should not
	// terminate. If they terminate they will be restarted with exponential
	// backoff for up to 5 minutes before the supervsior marks them as
	// failed and terminates all jobs.
	Main []*Command `json:"main"`
}

type EnvConfig struct {
	// PassAllVariables will pass all environment variables from the
	// supervisor environment through to the subprocess. VaultReplacements
	// and VaultTemplateVariables will still be respected. This implies
	// that PassVariables will be ignored.
	PassAllVariables bool `json:"pass-all"`

	// PassVariables indicates the variables from the supervisor
	// environment that should be passed through to the subprocess
	// environment. Only these and the vault variables below will be passed
	// through.
	PassVariables []string `json:"pass"`

	// SetVaultToken requests that VAULT_TOKEN be set in the subprocess
	// environment with a login token that the supervisor has obtained.
	// Setting this implies that VAULT_ADDR will also be passed through.
	SetVaultToken bool `json:"vault-token"`

	// VaultReplacements will be read from the supervisor environment and
	// treated as paths in vault. They will be read from vault and exported
	// into the subprocess environment under the same environment variable
	// name. The backing secrets will be renewed by the supervisor.
	//
	// A variable name being in this slice does not imply its membership in
	// PassVariables.
	//
	// Variable values should have the form type:path:field. Where type
	// is one of db or secret, path is the vault path, and field is the
	// name of the field in the returned JSON or Username/Password (case
	// sensitive) for db types.
	//
	// Secrets will only be fetched once and their fields re-used. So it
	// is possible to get one db session and place its username in one
	// variable and its password in another (for example, to use in a
	// template).
	VaultReplacements []string `json:"vault-replace"`

	// VaultTemplateVariables are variables in text/template format
	// that recursively refer to variables in VaultReplacements. These
	// can be used to, for example, template a jdbc string that will be
	// expanded by Vault before injection into the subprocess environment.
	// It is an error to refer to a variable that is not included in
	// VaultReplacements.
	//
	// A variable name being in this slice does not imply its membership in
	// PassVariables.
	VaultTemplateVariables []string `json:"vault-template"`
}

type Command struct {
	Name       string   `json:"name"`
	Command    []string `json:"cmd"`
	RunAsUser  string
	RunAsGroup string
	KillSignal syscall.Signal
}

func (c *Command) UnmarshalJSON(d []byte) error {
	type Alias Command

	cfg := struct {
		KillSig string `json:"kill-signal"`
		RunAs   string `json:"run-as"`
		*Alias
	}{Alias: (*Alias)(c)}

	if err := json.Unmarshal(d, &cfg); err != nil {
		return err
	}

	var ok bool
	if cfg.KillSig != "" {
		if c.KillSignal, ok = signalMap[cfg.KillSig]; !ok {
			return fmt.Errorf("Command.UnmarshalJSON: invalid signal %s", cfg.KillSig)
		}
	} else {
		c.KillSignal = syscall.SIGKILL
	}

	switch userGroup := strings.Split(cfg.RunAs, ":"); len(userGroup) {
	case 1:
		if userGroup[0] == "" {
			c.RunAsUser = "root"
			c.RunAsGroup = "root"
		} else {
			c.RunAsUser = userGroup[0]
			c.RunAsGroup = "root"
		}
	case 2:
		c.RunAsUser = userGroup[0]
		c.RunAsGroup = userGroup[1]
	default:
		return fmt.Errorf("Command.UnmarshalJSON: invalid run-as string %s", cfg.RunAs)
	}

	if c.Name == "" && len(c.Command) > 0 {
		c.Name = path.Base(c.Command[0])
	}

	return nil
}
