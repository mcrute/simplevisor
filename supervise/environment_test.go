package supervise

import (
	"context"
	"fmt"
	"testing"

	"code.crute.us/mcrute/golib/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type EnvListSuite struct {
	suite.Suite
	l *EnvList
}

func (s *EnvListSuite) SetupTest() {
	s.l = &EnvList{}
}

func (s *EnvListSuite) TestPut() {
	s.l.Put("foo", "bar")
	assert.Equal(s.T(), "foo=bar", (*s.l)[0])
}

func (s *EnvListSuite) TestPutAll() {
	s.l.PutAll(map[string]string{
		"biz": "buz",
		"baz": "bif",
	})
	assert.Contains(s.T(), *s.l, "biz=buz", "biz=buz")
}

func (s *EnvListSuite) TestPutSome() {
	s.l.PutSome(map[string]string{
		"blah": "hah",
		"fah":  "mah",
		"nah":  "tah",
	}, []string{"blah", "fah"})
	assert.Contains(s.T(), *s.l, "blah=hah", "fah=mah")
	assert.NotContains(s.T(), *s.l, "nah=tah")
}

func TestEnvList(t *testing.T) {
	suite.Run(t, &EnvListSuite{})
}

func TestGetEnvMap(t *testing.T) {
	envGetter = func() []string {
		return []string{
			"FOO=bar",
			"bar=baz",
			"BIZ=buz=baz=bup",
		}
	}

	m := getEnvMap()
	assert.Equal(t, "bar", m["FOO"])
	assert.Equal(t, "baz", m["bar"])
	assert.Equal(t, "buz=baz=bup", m["BIZ"])
}

func TestParseSecretId(t *testing.T) {
	_, _, _, err := parseSecretId("name", "foo:bar")
	assert.ErrorContains(t, err, "error parsing vault variable name")

	a, b, c, err := parseSecretId("foo", "db:path:key")
	assert.NoError(t, err)
	assert.Equal(t, "db", a)
	assert.Equal(t, "path", b)
	assert.Equal(t, "key", c)
}

type ProcessTemplatesSuite struct {
	suite.Suite
	env          map[string]string
	replacements map[string]string
}

func (s *ProcessTemplatesSuite) SetupTest() {
	s.env = map[string]string{
		"INVALID_TEMPLATE": "this:{{ is }}invalid",
		"VALID_TEMPLATE":   "this:{{ .FOO }}:is:{{ .BIZ }}",
		"VALID_TEMPLATE_2": "some:{{ .FOO }}",
	}

	s.replacements = map[string]string{
		"FOO": "BAR",
		"BIZ": "BAZ",
	}
}

func (s *ProcessTemplatesSuite) TestInvalidTemplate() {
	err := processTemplates(s.env, []string{"INVALID_TEMPLATE"}, s.replacements)
	assert.ErrorContains(s.T(), err, "template failed to parse for INVALID_TEMPLATE:")
}

func (s *ProcessTemplatesSuite) TestValidTemplate() {
	err := processTemplates(s.env, []string{"VALID_TEMPLATE"}, s.replacements)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "this:BAR:is:BAZ", s.env["VALID_TEMPLATE"])

	err = processTemplates(s.env, []string{"VALID_TEMPLATE_2"}, s.replacements)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "some:BAR", s.env["VALID_TEMPLATE_2"])
}

func TestProcessTemplates(t *testing.T) {
	suite.Run(t, &ProcessTemplatesSuite{})
}

type MockSecretClient struct {
	dbCalls     int
	secretCalls int
	doError     bool
}

func (c *MockSecretClient) DatabaseCredential(ctx context.Context, path string) (*secrets.Credential, secrets.Handle, error) {
	c.dbCalls += 1
	if c.doError {
		return nil, nil, fmt.Errorf("an error")
	}
	return map[string]*secrets.Credential{
		"path":  &secrets.Credential{Username: "user1", Password: "pass1"},
		"path2": &secrets.Credential{Username: "user2", Password: "pass2"},
	}[path], nil, nil
}
func (c *MockSecretClient) Secret(ctx context.Context, path string, out any) (secrets.Handle, error) {
	c.secretCalls += 1
	if c.doError {
		return nil, fmt.Errorf("an error")
	}
	o := out.(*map[string]string)
	switch path {
	case "path":
		(*o)["foo"] = "bar"
		(*o)["baz"] = "buz"
	case "path2":
		(*o)["biz"] = "buz"
	}
	return nil, nil
}
func (c *MockSecretClient) WriteSecret(ctx context.Context, path string, in any) error { return nil }
func (c *MockSecretClient) Destroy(h secrets.Handle) error                             { return nil }
func (c *MockSecretClient) MakeNonCritical(h secrets.Handle) error                     { return nil }

type ExpandReplacementsSuite struct {
	suite.Suite
	ctx context.Context
	sc  *MockSecretClient
	env map[string]string
}

func (s *ExpandReplacementsSuite) SetupTest() {
	s.env = map[string]string{
		"DB_SECRET_USERNAME":   "db:path:Username",
		"DB_SECRET_PASS":       "db:path:Password",
		"DB_SECRET_INVALID":    "db:path:invalid",
		"DB_SECRET_2_USERNAME": "db:path2:Username",
		"SECRET_KEY":           "secret:path:foo",
		"SECRET_KEY_ALSO":      "secret:path:baz",
		"SECRET_KEY_2":         "secret:path2:biz",
		"SECRET_KEY_INVALID":   "secret:path:invalid",
		"INVALID_TYPE":         "foo:bar:baz",
	}
	s.sc = &MockSecretClient{}
	s.ctx = context.TODO()
}

func (s *ExpandReplacementsSuite) TestDb() {
	r, err := expandReplacements(s.ctx, s.sc, s.env, []string{"DB_SECRET_USERNAME", "DB_SECRET_2_USERNAME"})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), map[string]string{
		"DB_SECRET_USERNAME":   "user1",
		"DB_SECRET_2_USERNAME": "user2",
	}, r)
	assert.Equal(s.T(), 2, s.sc.dbCalls)
}

func (s *ExpandReplacementsSuite) TestDbInvalidField() {
	_, err := expandReplacements(s.ctx, s.sc, s.env, []string{"DB_SECRET_INVALID"})
	assert.ErrorContains(s.T(), err, "unknown field invalid for db credential")
}

func (s *ExpandReplacementsSuite) TestDbPasswordAndCache() {
	r, err := expandReplacements(s.ctx, s.sc, s.env, []string{"DB_SECRET_USERNAME", "DB_SECRET_PASS"})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), map[string]string{
		"DB_SECRET_USERNAME": "user1",
		"DB_SECRET_PASS":     "pass1",
	}, r)
	assert.Equal(s.T(), 1, s.sc.dbCalls)
}

func (s *ExpandReplacementsSuite) TestSecret() {
	r, err := expandReplacements(s.ctx, s.sc, s.env, []string{"SECRET_KEY", "SECRET_KEY_2"})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), map[string]string{
		"SECRET_KEY":   "bar",
		"SECRET_KEY_2": "buz",
	}, r)
	assert.Equal(s.T(), 2, s.sc.secretCalls)
}

func (s *ExpandReplacementsSuite) TestSecretInvalidField() {
	_, err := expandReplacements(s.ctx, s.sc, s.env, []string{"SECRET_KEY_INVALID"})
	assert.ErrorContains(s.T(), err, "secret path has no field invalid")
}

func (s *ExpandReplacementsSuite) TestSecretCache() {
	r, err := expandReplacements(s.ctx, s.sc, s.env, []string{"SECRET_KEY", "SECRET_KEY_ALSO"})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), map[string]string{
		"SECRET_KEY":      "bar",
		"SECRET_KEY_ALSO": "buz",
	}, r)
	assert.Equal(s.T(), 1, s.sc.secretCalls)
}

func (s *ExpandReplacementsSuite) TestNotInEnv() {
	r, err := expandReplacements(s.ctx, s.sc, s.env, []string{"NOT_THERE"})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), map[string]string{}, r)
}

func (s *ExpandReplacementsSuite) TestInvalidType() {
	_, err := expandReplacements(s.ctx, s.sc, s.env, []string{"INVALID_TYPE"})
	assert.ErrorContains(s.T(), err, "invalid secret type foo")
}

func (s *ExpandReplacementsSuite) TestVaultErrors() {
	s.sc.doError = true
	_, err := expandReplacements(s.ctx, s.sc, s.env, []string{"DB_SECRET_USERNAME"})
	assert.ErrorContains(s.T(), err, "vault error: an error")

	_, err = expandReplacements(s.ctx, s.sc, s.env, []string{"SECRET_KEY"})
	assert.ErrorContains(s.T(), err, "vault error: an error")
}

func TestExpandReplacementsSuite(t *testing.T) {
	suite.Run(t, &ExpandReplacementsSuite{})
}

type PrepareEnvironmentSuite struct {
	suite.Suite
	sc  *MockSecretClient
	ctx context.Context
}

func (s *PrepareEnvironmentSuite) SetupTest() {
	s.ctx = context.TODO()
	s.sc = &MockSecretClient{}

	envGetter = func() []string {
		return []string{
			"FOO=bar",
			"BIZ=baz",
			"BUZ=bap",
		}
	}
}

func (s *PrepareEnvironmentSuite) TestVaultToken() {
	envGetter = func() []string { return []string{"VAULT_ADDR=addr"} }

	r, err := PrepareEnvironment(s.ctx, &EnvConfig{SetVaultToken: true}, s.sc, "token")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), r, "VAULT_TOKEN=token")
	assert.Contains(s.T(), r, "VAULT_ADDR=addr")

	envGetter = func() []string { return []string{} }

	r, err = PrepareEnvironment(s.ctx, &EnvConfig{SetVaultToken: true}, s.sc, "token")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), r, "VAULT_TOKEN=token")
	assert.NotContains(s.T(), r, "VAULT_ADDR=addr")

	r, err = PrepareEnvironment(s.ctx, &EnvConfig{SetVaultToken: true}, s.sc, "")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), []string{}, r)
}

func (s *PrepareEnvironmentSuite) TestPassAll() {
	r, err := PrepareEnvironment(s.ctx, &EnvConfig{PassAllVariables: true}, s.sc, "")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), r, "FOO=bar")
	assert.Contains(s.T(), r, "BIZ=baz")
	assert.Contains(s.T(), r, "BUZ=bap")
}

func (s *PrepareEnvironmentSuite) TestPassSome() {
	r, err := PrepareEnvironment(s.ctx, &EnvConfig{PassVariables: []string{"FOO", "BIZ"}}, s.sc, "")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), r, "FOO=bar")
	assert.Contains(s.T(), r, "BIZ=baz")
	assert.NotContains(s.T(), r, "BUZ=bap")
}

func TestPrepareEnvironmentSuite(t *testing.T) {
	suite.Run(t, &PrepareEnvironmentSuite{})
}
