package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

// Each Environment has a base URL; Production is the default.
var environments = map[string]string{
	"prod": "https://skipli.schifferarchitecture.com",
	"dev":  "https://dev.skipli.schifferarchitecture.com",
}

// session is what the CLI keeps between runs: the cookie values a browser
// would hold, or, between `login <email>` and `login --code`, the sign-in
// waiting for its code.
type session struct {
	IDToken      string     `json:"idToken,omitempty"`
	RefreshToken string     `json:"refreshToken,omitempty"`
	Challenge    *challenge `json:"challenge,omitempty"`
}

type challenge struct {
	Email   string `json:"email"`
	Session string `json:"session"`
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sessionFile names the file for a base URL: the Environment's name, or for
// any other URL (a local API) its host.
func sessionFile(dir, env, base string) string {
	name := env
	if name == "" {
		u, err := url.Parse(base)
		name = "url-" + unsafeName.ReplaceAllString(base, "_")
		if err == nil && u.Host != "" {
			name = "url-" + unsafeName.ReplaceAllString(u.Host, "_")
		}
	}
	return filepath.Join(dir, name+".json")
}

func loadSession(path string) (session, error) {
	var s session
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return session{}, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

func saveSession(path string, s session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func deleteSession(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
