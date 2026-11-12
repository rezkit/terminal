// Package auth implements the OAuth 2.0 Authorization Code + PKCE flow
// against Logto, with a loopback redirect and persistent token storage.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Discovery is the subset of OpenID provider metadata we need.
type Discovery struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	EndSessionEndpoint          string `json:"end_session_endpoint"`
	RevocationEndpoint          string `json:"revocation_endpoint"`
}

// Discover fetches {issuer}/.well-known/openid-configuration.
func Discover(ctx context.Context, client *http.Client, issuer string) (*Discovery, error) {
	url := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching OIDC discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching OIDC discovery: %s", resp.Status)
	}
	var d Discovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decoding OIDC discovery: %w", err)
	}
	if d.TokenEndpoint == "" {
		return nil, fmt.Errorf("OIDC discovery at %s lacks a token endpoint", url)
	}
	return &d, nil
}
