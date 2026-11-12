package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// ErrReauthRequired means the refresh token is missing, expired or revoked
// and the user must log in again.
var ErrReauthRequired = errors.New("session expired; please log in again")

// expiryLeeway refreshes slightly before the access token actually expires.
const expiryLeeway = 30 * time.Second

// TokenSource returns an oauth2.TokenSource that refreshes with the Logto
// resource indicator (the stock oauth2 refresher cannot send it, and without
// it Logto returns an opaque token the API rejects) and saves rotated tokens
// to the store.
func (a *Authenticator) TokenSource(initial *oauth2.Token, store Store) oauth2.TokenSource {
	return &refreshingSource{auth: a, tok: initial, store: store}
}

type refreshingSource struct {
	auth  *Authenticator
	store Store

	mu  sync.Mutex
	tok *oauth2.Token
}

func (s *refreshingSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tok != nil && s.tok.AccessToken != "" &&
		(s.tok.Expiry.IsZero() || time.Until(s.tok.Expiry) > expiryLeeway) {
		return s.tok, nil
	}
	if s.tok == nil || s.tok.RefreshToken == "" {
		return nil, ErrReauthRequired
	}
	tok, err := s.auth.refresh(context.Background(), s.tok.RefreshToken)
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" { // not rotated: keep the old one
		tok.RefreshToken = s.tok.RefreshToken
	}
	if tok.Extra("id_token") == nil { // keep the old ID token for display
		if id, ok := s.tok.Extra("id_token").(string); ok {
			tok = tok.WithExtra(map[string]any{"id_token": id})
		}
	}
	s.tok = tok
	if s.store != nil {
		if err := s.store.Save(tok); err != nil {
			return nil, fmt.Errorf("saving refreshed token: %w", err)
		}
	}
	return tok, nil
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	IDToken          string `json:"id_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (a *Authenticator) refresh(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {a.opts.ClientID},
	}
	if a.opts.Resource != "" {
		form.Set("resource", a.opts.Resource)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.disc.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("refreshing token: decoding response (%s): %w", resp.Status, err)
	}
	if resp.StatusCode != http.StatusOK || tr.Error != "" {
		// invalid_grant (expired/revoked), invalid_target (resource never
		// granted, e.g. resource configured after login), invalid_scope...:
		// all fixed by logging in again.
		if strings.HasPrefix(tr.Error, "invalid_") {
			return nil, fmt.Errorf("%w (%s: %s)", ErrReauthRequired, tr.Error, tr.ErrorDescription)
		}
		return nil, fmt.Errorf("refreshing token: %s: %s %s", resp.Status, tr.Error, tr.ErrorDescription)
	}
	tok := &oauth2.Token{
		AccessToken:  tr.AccessToken,
		TokenType:    tr.TokenType,
		RefreshToken: tr.RefreshToken,
	}
	if tr.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	if tr.IDToken != "" {
		tok = tok.WithExtra(map[string]any{"id_token": tr.IDToken})
	}
	return tok, nil
}

// StoreTokenSource returns a TokenSource that reads the token from store
// when first needed (and again after a failure), so a login completed after
// the source was created is picked up without restarting.
func (a *Authenticator) StoreTokenSource(store Store) oauth2.TokenSource {
	return &storeSource{auth: a, store: store}
}

type storeSource struct {
	auth  *Authenticator
	store Store

	mu    sync.Mutex
	inner oauth2.TokenSource
}

func (s *storeSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inner == nil {
		tok, err := s.store.Load()
		if err != nil {
			return nil, err
		}
		s.inner = s.auth.TokenSource(tok, s.store)
	}
	tok, err := s.inner.Token()
	if err != nil {
		s.inner = nil // reload from the store next time
	}
	return tok, err
}
