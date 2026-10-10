package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// credentials is the key that lillamolnet login saved, and the API it is for.
type credentials struct {
	APIURL    string    `json:"api_url"`
	APIKey    string    `json:"api_key"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// errNoConfigDir means the environment names no directory for the credentials.
var errNoConfigDir = errors.New("no place for the credentials: set HOME or XDG_CONFIG_HOME")

// credentialsPath is $XDG_CONFIG_HOME/lillamolnet/credentials,
// ~/.config/lillamolnet/credentials, or %APPDATA%\lillamolnet\credentials on
// Windows. Only the environment decides, so that tests never touch the real
// file.
func credentialsPath(getenv func(string) string) (string, error) {
	if dir := getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "lillamolnet", "credentials"), nil
	}
	if home := getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "lillamolnet", "credentials"), nil
	}
	if appData := getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "lillamolnet", "credentials"), nil
	}
	return "", errNoConfigDir
}

// loadCredentials returns the saved key, or nil when there is none.
func loadCredentials(getenv func(string) string) (*credentials, error) {
	path, err := credentialsPath(getenv)
	if errors.Is(err, errNoConfigDir) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var c credentials
	if err := json.Unmarshal(data, &c); err != nil || c.APIKey == "" {
		return nil, fmt.Errorf("%s is not valid. Run lillamolnet login again", path)
	}
	return &c, nil
}

// saveCredentials writes the key so that only the user can read it.
func saveCredentials(getenv func(string) string, c credentials) (string, error) {
	path, err := credentialsPath(getenv)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}

	// Write a new file and move it in place, so that a half-written file
	// never replaces a good one.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}

// removeCredentials deletes the saved key, if there is one.
func removeCredentials(getenv func(string) string) error {
	path, err := credentialsPath(getenv)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
