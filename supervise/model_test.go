package supervise

import (
	"encoding/json"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalCommandValidKillSignal(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo", "kill-signal": "KILL"}`)
	cmd := &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, []string{"test"}, cmd.Command)
	assert.Equal(t, "foo", cmd.RunAsUser)
	assert.Equal(t, syscall.SIGKILL, cmd.KillSignal)
}

func TestUnmarshalCommandInvalidKillSignal(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo", "kill-signal": "FOO"}`)
	cmd := &Command{}

	assert.ErrorContains(t, json.Unmarshal(cfg, &cmd), "invalid signal")
}

func TestUnmarshalCommandNoKillSignal(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo"}`)
	cmd := &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, syscall.SIGKILL, cmd.KillSignal)
}

func TestUnmarshalCommandNoUser(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "kill-signal": "KILL"}`)
	cmd := &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "root", cmd.RunAsUser)
	assert.Equal(t, "root", cmd.RunAsGroup)
}

func TestUnmarshalCommandUserOnly(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo", "kill-signal": "KILL"}`)
	cmd := &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "foo", cmd.RunAsUser)
	assert.Equal(t, "root", cmd.RunAsGroup)
}

func TestUnmarshalCommandUserGroup(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo:bar", "kill-signal": "KILL"}`)
	cmd := &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "foo", cmd.RunAsUser)
	assert.Equal(t, "bar", cmd.RunAsGroup)
}

func TestUnmarshalCommandInvalidUser(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo:bar:baz", "kill-signal": "KILL"}`)
	cmd := &Command{}

	assert.ErrorContains(t, json.Unmarshal(cfg, &cmd), "invalid run-as string")
}

func TestUnmarshalCommandName(t *testing.T) {
	cfg := []byte(`{"cmd": ["test"], "run-as": "foo", "kill-signal": "KILL"}`)
	cmd := &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "test", cmd.Name)

	cfg = []byte(`{"cmd": ["/foo/bar/test"], "run-as": "foo", "kill-signal": "KILL"}`)
	cmd = &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "test", cmd.Name)

	cfg = []byte(`{"name": "foop", "cmd": ["/foo/bar/test"], "run-as": "foo", "kill-signal": "KILL"}`)
	cmd = &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "foop", cmd.Name)

	cfg = []byte(`{"run-as": "foo", "kill-signal": "KILL"}`)
	cmd = &Command{}

	assert.NoError(t, json.Unmarshal(cfg, &cmd))
	assert.Equal(t, "", cmd.Name)
}
