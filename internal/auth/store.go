package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// ErrNoToken is returned when no token has been stored.
var ErrNoToken = errors.New("not logged in")

// Store persists OAuth tokens.
type Store interface {
	Load() (*oauth2.Token, error)
	Save(*oauth2.Token) error
	Delete() error
}

const keyringService = "rezkit-tui"

// KeyringStore keeps the token in the OS keychain (Secret Service, macOS
// Keychain, Windows Credential Manager).
type KeyringStore struct{ User string }

func (k KeyringStore) Load() (*oauth2.Token, error) {
	s, err := keyring.Get(keyringService, k.User)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNoToken
	}
	if err != nil {
		return nil, err
	}
	return decodeToken([]byte(s))
}

func (k KeyringStore) Save(t *oauth2.Token) error {
	b, err := encodeToken(t)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, k.User, string(b))
}

func (k KeyringStore) Delete() error {
	err := keyring.Delete(keyringService, k.User)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// FileStore keeps the token in a 0600 JSON file. Used where no keychain is
// available (e.g. WSL without a Secret Service daemon).
type FileStore struct{ Path string }

func (f FileStore) Load() (*oauth2.Token, error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoToken
	}
	if err != nil {
		return nil, err
	}
	return decodeToken(b)
}

func (f FileStore) Save(t *oauth2.Token) error {
	b, err := encodeToken(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}

func (f FileStore) Delete() error {
	err := os.Remove(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// storedToken adds the ID token, which oauth2.Token keeps in an unexported
// field that json.Marshal would drop.
type storedToken struct {
	Token   *oauth2.Token `json:"token"`
	IDToken string        `json:"id_token,omitempty"`
}

func encodeToken(t *oauth2.Token) ([]byte, error) {
	id, _ := t.Extra("id_token").(string)
	return json.Marshal(storedToken{Token: t, IDToken: id})
}

func decodeToken(b []byte) (*oauth2.Token, error) {
	var s storedToken
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("stored token is corrupt: %w", err)
	}
	if s.Token == nil {
		return nil, errors.New("stored token is corrupt: no token")
	}
	if s.IDToken != "" {
		return s.Token.WithExtra(map[string]any{"id_token": s.IDToken}), nil
	}
	return s.Token, nil
}

// fallbackStore tries the keychain and falls back to a file when the keychain
// is unavailable. Reads check both so a token is found wherever it was saved.
type fallbackStore struct {
	primary  Store
	fallback Store
}

// NewStore returns a keychain-backed store that falls back to a file in dir.
func NewStore(user, dir string) Store {
	return fallbackStore{
		primary:  KeyringStore{User: user},
		fallback: FileStore{Path: filepath.Join(dir, "token.json")},
	}
}

func (s fallbackStore) Load() (*oauth2.Token, error) {
	t, err := s.primary.Load()
	if err == nil {
		return t, nil
	}
	return s.fallback.Load()
}

func (s fallbackStore) Save(t *oauth2.Token) error {
	if err := s.primary.Save(t); err == nil {
		_ = s.fallback.Delete() // don't leave a stale copy on disk
		return nil
	}
	return s.fallback.Save(t)
}

// Delete removes the token from both places. Keychain errors are ignored
// because "no keychain available" is the normal case on WSL.
func (s fallbackStore) Delete() error {
	_ = s.primary.Delete()
	return s.fallback.Delete()
}
