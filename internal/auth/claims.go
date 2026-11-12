package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"golang.org/x/oauth2"
)

// DisplayName extracts a human-readable name from the token's ID token, for
// display only. The ID token is NOT verified here; never use this for
// authorization decisions.
func DisplayName(tok *oauth2.Token) string {
	if tok == nil {
		return ""
	}
	raw, _ := tok.Extra("id_token").(string)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var c struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if json.Unmarshal(payload, &c) != nil {
		return ""
	}
	for _, s := range []string{c.Name, c.Username, c.Email} {
		if s != "" {
			return s
		}
	}
	return ""
}
