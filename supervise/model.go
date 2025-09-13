package supervise

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
)

//go:generate go run ../generate_syscall/main.go

type RestartPolicy string

const (
	Always        RestartPolicy = "always"
	Never                       = "never"
	UnlessSuccess               = "unless-success"
)

type AppConfig struct {
	Environment *EnvConfig  `json:"env"`
	Jobs        *JobsConfig `json:"jobs"`
}

func ReadAppConfig(path string) (*AppConfig, error) {
	cf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("readConfig: unable to load config")
	}

	cfg := &AppConfig{}
	if err := json.Unmarshal(cf, &cfg); err != nil {
		return nil, fmt.Errorf("readConfig: unable to parse config: %s", err)
	}

	return cfg, nil
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
	// Name is the name of the command. This is used for logging purposes.
	Name string `json:"name"`

	// Command is an array containing the command name and arguments.
	// If the command name is not prefixed with a path then it will be
	// searched in the PATH set in the environment.
	Command []string `json:"cmd"`

	// PassVariables indicates the variables from the supervisor
	// environment that should be passed through to the subprocess
	// environment. Only these and the vault variables below will be passed
	// through. The default behavior, if this is not specified, is to pass
	// all variables from the supervisor pass configuration through to the
	// process.
	PassVariables []string `json:"pass"`

	// LogsJson indicates to the process supervisor that the log lines
	// emitted by this process are already in JSON format. This currently
	// does nothing.
	LogsJson bool `json:"logs-json"`

	// SuccessLifetime is the amount of time that the job must run to be
	// considered successfully started. Anything less than this amount
	// of time will be considered a failure to start and counted against
	// RestartMaxRetries if restarting is enabled. If the job runs for at
	// least this amount of time it will reset the restart counter. Default
	// is 1 minute.
	SuccessLifetime time.Duration

	// RestartPolicy is the policy the supervisor will apply to job
	// termination. The default for main jobs is "always", meaning the
	// job will always be restarted if it stops running. Setting this
	// to anything other than "never" for an init job is an error.
	//
	// Available policies are:
	// - always: always restart the job if it stops running
	// - never: never restart the job if it stops running
	// - unless-success: restart the job only if the exit code is non-zero
	RestartPolicy RestartPolicy `json:"restart-policy"`

	// RestartMaxRetries is the number of failed attempts to restart a job
	// before the supervisor considers it to be failed. This counter is
	// reset to zero upon each successful job start. The default is -1,
	// meaning no limit to retries.
	RestartMaxRetries int

	// RestartMaxTime the total amount of time allowed to attempt to
	// restart the job before the job is considered failed by the
	// supervisor. The supervisor will use an expoentially increasing
	// back-off strategy for restarting the job. The default is 1 hour.
	RestartMaxTime time.Duration

	// Critical indicates that the job is considered to be critical and
	// the failure to start or restart the job will result in the process
	// supervisor terminating with an error. The default is true for init
	// jobs and false otherwise.
	Critical *bool `json:"critical"`

	// RunAsUser and RunAsGroup are the user and group as which to start
	// the job. These are specified in the `run-as` configuration stanza
	// which takes the form user or user:group. If not specified the
	// default is root:root.
	RunAsUser  string
	RunAsGroup string

	// KillSignal is the name of a Unix singnal (without the SIG prefix) to
	// send to the process upon clean termination. The default is KILL.
	KillSignal syscall.Signal
}

func (c *Command) UnmarshalJSON(d []byte) error {
	type Alias Command

	cfg := struct {
		KillSig           string         `json:"kill-signal"`
		RunAs             string         `json:"run-as"`
		RestartMaxRetries *int           `json:"restart-max-retries"`
		RestartMaxTime    *time.Duration `json:"restart-max-time"`
		SuccessLifetime   *time.Duration `json:"success-lifetime"`
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

	if cfg.RestartMaxRetries != nil {
		c.RestartMaxRetries = *cfg.RestartMaxRetries
	} else {
		c.RestartMaxRetries = -1
	}

	if cfg.RestartMaxTime != nil {
		c.RestartMaxTime = *cfg.RestartMaxTime
	} else {
		c.RestartMaxTime = time.Hour
	}

	if cfg.SuccessLifetime != nil {
		c.SuccessLifetime = *cfg.SuccessLifetime
	} else {
		c.SuccessLifetime = time.Minute
	}

	return nil
}
