package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/suessflorian/gqlfetch"
	"golang.org/x/oauth2"
)

// NewHTTPClient builds an HTTP client that adds a bearer token from ts to
// every request.
func NewHTTPClient(ts oauth2.TokenSource) *http.Client {
	return &http.Client{
		Transport: &oauth2.Transport{Source: ts, Base: http.DefaultTransport},
		Timeout:   30 * time.Second,
	}
}

// FetchSchema runs an introspection query and returns the schema as SDL.
// ts may be nil: the RezKit API allows anonymous introspection.
func FetchSchema(ctx context.Context, endpoint string, ts oauth2.TokenSource) (string, error) {
	h := http.Header{}
	if ts != nil {
		tok, err := ts.Token()
		if err != nil {
			return "", err
		}
		tok.SetAuthHeader(&http.Request{Header: h})
	}
	sdl, err := gqlfetch.BuildClientSchemaWithHeaders(ctx, endpoint, h, true)
	if err != nil {
		return "", fmt.Errorf("introspecting %s: %w", endpoint, err)
	}
	return sdl, nil
}
