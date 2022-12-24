package supervise

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"text/template"

	"code.crute.us/mcrute/golib/secrets"
)

// For testing purposes only
var envGetter func() []string = os.Environ

type EnvList []string

func (l *EnvList) Put(k, v string) {
	*l = append(*l, fmt.Sprintf("%s=%s", k, v))
}

func (l *EnvList) PutAll(m map[string]string) {
	for k, v := range m {
		l.Put(k, v)
	}
}

func (l *EnvList) PutSome(m map[string]string, w []string) {
	for _, k := range w {
		v, ok := m[k]
		if ok {
			l.Put(k, v)
		}
	}
}

func getEnvMap() map[string]string {
	r := envGetter()
	m := make(map[string]string, len(r))

	for _, v := range r {
		kv := strings.SplitN(v, "=", 2)
		m[kv[0]] = kv[1]
	}

	return m
}

func processTemplates(env map[string]string, vars []string, replacements map[string]string) (err error) {
	var tpl *template.Template
	buf := &bytes.Buffer{}
	for _, k := range vars {
		if v, ok := env[k]; ok {
			if tpl, err = template.New("").Parse(v); err != nil {
				return fmt.Errorf("PrepareEnvironment: template failed to parse for %s: %w", k, err)
			}

			if err = tpl.Execute(buf, replacements); err != nil {
				return fmt.Errorf("PrepareEnvironment: error processing template %s: %w", k, err)
			}

			env[k] = buf.String()
			buf.Reset()
		}
	}
	return nil
}

func parseSecretId(name, id string) (string, string, string, error) {
	parsed := strings.Split(id, ":")
	if len(parsed) != 3 {
		return "", "", "", fmt.Errorf("PrepareEnvironment: error parsing vault variable %s, not len(3)", name)
	}
	return parsed[0], parsed[1], parsed[2], nil
}

func expandReplacements(ctx context.Context, sc secrets.Client, envMap map[string]string, keys []string) (map[string]string, error) {
	// Pare this down to just VaultReplacements for template expansion
	replacements := make(map[string]string, len(keys))

	dbCache := map[string]*secrets.Credential{}
	secretCache := map[string]map[string]string{}
	awsUserCache := map[string]*secrets.AWSCredential{}
	for _, k := range keys {
		v, ok := envMap[k]
		if !ok {
			continue
		}

		secretType, secretPath, jsonField, err := parseSecretId(k, v)
		if err != nil {
			return nil, err
		}

		switch secretType {
		case "db":
			cred, ok := dbCache[secretPath]
			if !ok {
				cred, _, err = sc.DatabaseCredential(ctx, secretPath)
				if err != nil {
					return nil, fmt.Errorf("PrepareEnvironment: vault error: %w", err)
				}
				dbCache[secretPath] = cred
			}

			switch jsonField {
			case "Username":
				envMap[k] = cred.Username
				replacements[k] = cred.Username
			case "Password":
				envMap[k] = cred.Password
				replacements[k] = cred.Password
			default:
				return nil, fmt.Errorf("PrepareEnvironment: unknown field %s for db credential", jsonField)
			}
		case "secret":
			s, ok := secretCache[secretPath]
			if !ok {
				s = map[string]string{}
				if _, err = sc.Secret(ctx, secretPath, &s); err != nil {
					return nil, fmt.Errorf("PrepareEnvironment: vault error: %w", err)
				}
				secretCache[secretPath] = s
			}

			val, ok := s[jsonField]
			if !ok {
				return nil, fmt.Errorf("PrepareEnvironment: secret %s has no field %s", secretPath, jsonField)
			}
			envMap[k] = val
			replacements[k] = val
		case "aws-user":
			cred, ok := awsUserCache[secretPath]
			if !ok {
				cred, _, err = sc.AWSIAMUser(ctx, secretPath)
				if err != nil {
					return nil, fmt.Errorf("PrepareEnvironment: vault error: %w", err)
				}
				awsUserCache[secretPath] = cred
			}

			switch jsonField {
			case "KeyId":
				envMap[k] = cred.AccessKeyId
				replacements[k] = cred.AccessKeyId
			case "SecretKey":
				envMap[k] = cred.SecretAccessKey
				replacements[k] = cred.SecretAccessKey
			default:
				return nil, fmt.Errorf("PrepareEnvironment: unknown field %s for AWS IAM user credential", jsonField)
			}
		default:
			return nil, fmt.Errorf("PrepareEnvironment: invalid secret type %s", secretType)
		}
	}

	return replacements, nil
}

func PrepareEnvironment(ctx context.Context, c *EnvConfig, sc secrets.Client, vaultToken string) ([]string, error) {
	var err error

	envMap := getEnvMap()
	out := EnvList{}

	// Export VAULT_TOKEN
	if c.SetVaultToken && vaultToken != "" {
		out.Put("VAULT_TOKEN", vaultToken)
		if va, ok := envMap["VAULT_ADDR"]; ok {
			out.Put("VAULT_ADDR", va)
		}
	}

	var replacements map[string]string

	// Process vault expansions
	if c.VaultReplacements != nil {
		if replacements, err = expandReplacements(ctx, sc, envMap, c.VaultReplacements); err != nil {
			return nil, err
		}
	}

	// Process templates
	if c.VaultTemplateVariables != nil && replacements != nil {
		if err = processTemplates(envMap, c.VaultTemplateVariables, replacements); err != nil {
			return nil, err
		}
	}

	// If we're passing everything then format it and return
	if c.PassAllVariables {
		out.PutAll(envMap)
	} else if c.PassVariables != nil {
		// Otherwise pass only the configured names
		out.PutSome(envMap, c.PassVariables)
	}

	return []string(out), nil
}
