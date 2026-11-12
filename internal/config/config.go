// Package config loads rezkit-tui settings from the user config directory,
// with environment variable overrides.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const appName = "rezkit-tui"

// Defaults for RezKit production. All values are public identifiers.
const (
	DefaultIssuer   = "https://auth.rezkit.app/oidc"
	DefaultClientID = "aeajwp2w42rdv4mcn7jki"
	DefaultResource = "https://api.rezkit.app/sales"
	DefaultEndpoint = "https://api.rezkit.app/api/graphql"
)

// Config holds everything needed to authenticate and talk to the API.
type Config struct {
	// Auth (Logto)
	Issuer   string   `toml:"issuer"`    // e.g. https://auth.example.com/oidc
	ClientID string   `toml:"client_id"` // Logto "Native" application ID
	Resource string   `toml:"resource"`  // API resource indicator
	Scopes   []string `toml:"scopes"`    // extra API scopes beyond openid/offline_access

	// API
	Endpoint string `toml:"endpoint"` // GraphQL HTTPS endpoint

	path string
}

// Dir returns the directory holding the config file and fallback token store.
func Dir() (string, error) {
	if d := os.Getenv("REZKIT_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating config dir: %w", err)
	}
	return filepath.Join(base, appName), nil
}

// Load reads config.toml (if present) and applies REZKIT_* overrides.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return LoadFrom(filepath.Join(dir, "config.toml"))
}

// LoadFrom reads the given file (missing is fine) and applies env overrides.
func LoadFrom(path string) (*Config, error) {
	c := &Config{path: path}
	if _, err := toml.DecodeFile(path, c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	c.applyEnv()
	if c.Issuer == "" {
		c.Issuer = DefaultIssuer
	}
	if c.ClientID == "" {
		c.ClientID = DefaultClientID
	}
	if c.Resource == "" {
		c.Resource = DefaultResource
	}
	if c.Endpoint == "" {
		c.Endpoint = DefaultEndpoint
	}
	return c, nil
}

func (c *Config) applyEnv() {
	str := func(name string, dst *string) {
		if v, ok := os.LookupEnv(name); ok {
			*dst = v
		}
	}
	str("REZKIT_ISSUER", &c.Issuer)
	str("REZKIT_CLIENT_ID", &c.ClientID)
	str("REZKIT_RESOURCE", &c.Resource)
	str("REZKIT_ENDPOINT", &c.Endpoint)
	if v := os.Getenv("REZKIT_SCOPES"); v != "" {
		c.Scopes = strings.Fields(strings.ReplaceAll(v, ",", " "))
	}
}

// Path returns the file this config was loaded from.
func (c *Config) Path() string { return c.path }

// ValidateAuth reports which auth settings are missing.
func (c *Config) ValidateAuth() error {
	var missing []string
	if c.Issuer == "" {
		missing = append(missing, "issuer")
	}
	if c.ClientID == "" {
		missing = append(missing, "client_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing auth settings %v in %s (or REZKIT_* env vars)", missing, c.path)
	}
	return nil
}

// ValidateAPI reports whether the API endpoint is configured.
func (c *Config) ValidateAPI() error {
	if c.Endpoint == "" {
		return fmt.Errorf("missing endpoint in %s (or REZKIT_ENDPOINT)", c.path)
	}
	return nil
}

// State is app-written data (as opposed to user-edited config). It lives in
// state.toml so the app never rewrites config.toml, which would freeze env
// overrides and built-in defaults into the user's file.
type State struct {
	LastWorkspace string `toml:"last_workspace"`

	path string
}

// LoadState reads state.toml from the config directory; missing is fine.
func LoadState() (*State, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	s := &State{path: filepath.Join(dir, "state.toml")}
	if _, err := toml.DecodeFile(s.path, s); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	return s, nil
}

// Save writes the state file, creating the directory if needed.
func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}
