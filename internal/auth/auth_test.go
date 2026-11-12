package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeLogto implements discovery, device authorization and the token endpoint.
type fakeLogto struct {
	t   *testing.T
	srv *httptest.Server

	mu            sync.Mutex
	challenge     string
	polls         int
	pendingPolls  int
	refreshForms  []map[string]string
	refreshStatus int
	refreshBody   map[string]any
}

func newFakeLogto(t *testing.T) *fakeLogto {
	f := &fakeLogto{t: t, pendingPolls: 1, refreshStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/oidc/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := f.srv.URL + "/oidc"
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                        base,
			"authorization_endpoint":        base + "/auth",
			"device_authorization_endpoint": base + "/device/auth",
			"token_endpoint":                base + "/token",
		})
	})
	mux.HandleFunc("/oidc/device/auth", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.challenge = r.PostForm.Get("code_challenge")
		f.mu.Unlock()
		if r.PostForm.Get("code_challenge_method") != "S256" {
			t.Errorf("code_challenge_method = %q", r.PostForm.Get("code_challenge_method"))
		}
		if r.PostForm.Get("resource") != "https://api.test" {
			t.Errorf("device auth resource = %q", r.PostForm.Get("resource"))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"device_code":      "dev-code",
			"user_code":        "ABCD-EFGH",
			"verification_uri": f.srv.URL + "/device",
			"expires_in":       600,
			"interval":         1,
		})
	})
	mux.HandleFunc("/oidc/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form := map[string]string{}
		for k := range r.PostForm {
			form[k] = r.PostForm.Get(k)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch form["grant_type"] {
		case "urn:ietf:params:oauth:grant-type:device_code":
			f.polls++
			if f.polls <= f.pendingPolls {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "authorization_pending"})
				return
			}
			sum := sha256.Sum256([]byte(form["code_verifier"]))
			if got := base64.RawURLEncoding.EncodeToString(sum[:]); got != f.challenge {
				t.Errorf("PKCE verifier does not match challenge")
			}
			if form["resource"] != "https://api.test" {
				t.Errorf("token resource = %q", form["resource"])
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"access_token": "at-1", "token_type": "Bearer",
				"refresh_token": "rt-1", "expires_in": 3600,
			})
		case "refresh_token":
			f.refreshForms = append(f.refreshForms, form)
			writeJSON(w, f.refreshStatus, f.refreshBody)
		default:
			t.Errorf("unexpected grant_type %q", form["grant_type"])
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func newTestAuth(t *testing.T, f *fakeLogto) *Authenticator {
	t.Helper()
	a, err := New(context.Background(), Options{
		Issuer:   f.srv.URL + "/oidc",
		ClientID: "client-123",
		Resource: "https://api.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDeviceLogin(t *testing.T) {
	f := newFakeLogto(t)
	a := newTestAuth(t, f)

	var shown DeviceCode
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tok, err := a.Login(ctx, func(dc DeviceCode) { shown = dc })
	if err != nil {
		t.Fatal(err)
	}
	if shown.UserCode != "ABCD-EFGH" {
		t.Errorf("user code = %q", shown.UserCode)
	}
	if tok.AccessToken != "at-1" || tok.RefreshToken != "rt-1" {
		t.Errorf("token = %+v", tok)
	}
	if f.polls != 2 {
		t.Errorf("polls = %d, want 2 (one pending, one success)", f.polls)
	}
}

func TestRefreshSendsResourceAndPersistsRotation(t *testing.T) {
	f := newFakeLogto(t)
	f.refreshBody = map[string]any{
		"access_token": "at-2", "token_type": "Bearer", "refresh_token": "rt-2", "expires_in": 3600,
	}
	a := newTestAuth(t, f)
	store := FileStore{Path: filepath.Join(t.TempDir(), "token.json")}

	expired := &oauth2.Token{AccessToken: "old", RefreshToken: "rt-1", Expiry: time.Now().Add(-time.Minute)}
	ts := a.TokenSource(expired, store)
	tok, err := ts.Token()
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at-2" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if len(f.refreshForms) != 1 || f.refreshForms[0]["resource"] != "https://api.test" ||
		f.refreshForms[0]["refresh_token"] != "rt-1" || f.refreshForms[0]["client_id"] != "client-123" {
		t.Errorf("refresh form = %+v", f.refreshForms)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != "rt-2" {
		t.Errorf("saved refresh token = %q, want rotated rt-2", saved.RefreshToken)
	}

	// A valid token must not trigger another refresh.
	if _, err := ts.Token(); err != nil {
		t.Fatal(err)
	}
	if len(f.refreshForms) != 1 {
		t.Errorf("refreshed again with a valid token")
	}
}

func TestRefreshInvalidGrantRequiresReauth(t *testing.T) {
	f := newFakeLogto(t)
	f.refreshStatus = http.StatusBadRequest
	f.refreshBody = map[string]any{"error": "invalid_grant"}
	a := newTestAuth(t, f)

	ts := a.TokenSource(&oauth2.Token{RefreshToken: "rt-x", Expiry: time.Now().Add(-time.Hour)}, nil)
	if _, err := ts.Token(); !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("err = %v, want ErrReauthRequired", err)
	}
	if _, err := a.TokenSource(nil, nil).Token(); !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("nil token: err = %v, want ErrReauthRequired", err)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	s := FileStore{Path: filepath.Join(t.TempDir(), "sub", "token.json")}
	if _, err := s.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty load err = %v", err)
	}
	if err := s.Save(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got.RefreshToken != "r" {
		t.Fatalf("load = %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestIDTokenSurvivesStoreAndRefresh(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"name":"Ada Lovelace"}`))
	idTok := "hdr." + payload + ".sig"
	s := FileStore{Path: filepath.Join(t.TempDir(), "token.json")}
	tok := (&oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(-time.Hour)}).
		WithExtra(map[string]any{"id_token": idTok})
	if err := s.Save(tok); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if name := DisplayName(got); name != "Ada Lovelace" {
		t.Fatalf("DisplayName after load = %q", name)
	}

	// A refresh response without an id_token must keep the old one.
	f := newFakeLogto(t)
	f.refreshBody = map[string]any{"access_token": "at-2", "token_type": "Bearer", "expires_in": 3600}
	refreshed, err := newTestAuth(t, f).TokenSource(got, s).Token()
	if err != nil {
		t.Fatal(err)
	}
	if DisplayName(refreshed) != "Ada Lovelace" {
		t.Fatal("ID token lost on refresh")
	}
	if reloaded, _ := s.Load(); DisplayName(reloaded) != "Ada Lovelace" || reloaded.RefreshToken != "r" {
		t.Fatalf("persisted after refresh: name=%q rt=%q", DisplayName(reloaded), reloaded.RefreshToken)
	}
}

func TestRefreshInvalidTargetRequiresReauth(t *testing.T) {
	f := newFakeLogto(t)
	f.refreshStatus = http.StatusBadRequest
	f.refreshBody = map[string]any{"error": "invalid_target", "error_description": "resource not granted"}
	ts := newTestAuth(t, f).TokenSource(&oauth2.Token{RefreshToken: "rt", Expiry: time.Now().Add(-time.Hour)}, nil)
	if _, err := ts.Token(); !errors.Is(err, ErrReauthRequired) {
		t.Fatalf("err = %v, want ErrReauthRequired", err)
	}
}
