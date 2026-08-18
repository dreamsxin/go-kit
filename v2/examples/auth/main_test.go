package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
)

func newAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := kit.MustNew(":0", kit.WithHTTPMiddleware(authenticate(apiKeys)))

	type meRequest struct{}
	kit.HandleJSONTyped(svc, "/api/me", func(ctx context.Context, _ meRequest) (identity, error) {
		id, ok := identityFromContext(ctx)
		if !ok {
			return identity{}, apperror.New(apperror.KindUnauthenticated, "auth.no_identity", "not authenticated")
		}
		return id, nil
	})
	svc.Handle("/api/admin", requireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"welcome"}`))
	})))

	return httptest.NewServer(svc)
}

func TestMissingCredentialsGet401(t *testing.T) {
	srv := newAuthServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/me", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "auth.bearer_required" {
		t.Errorf("code: got %q, want auth.bearer_required", body.Code)
	}
}

func TestUnknownCredentialsGet401(t *testing.T) {
	srv := newAuthServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/me", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer wrong-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestAuthenticatedCallerSeesIdentity(t *testing.T) {
	srv := newAuthServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/me", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer reader-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var id identity
	if err := json.NewDecoder(resp.Body).Decode(&id); err != nil {
		t.Fatal(err)
	}
	if id.Subject != "reader" {
		t.Errorf("subject: got %q, want reader", id.Subject)
	}
}

func TestReaderCannotReachAdminRoute(t *testing.T) {
	srv := newAuthServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/admin", nil)
	req.Header.Set("Authorization", "Bearer reader-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "auth.role_required" {
		t.Errorf("code: got %q, want auth.role_required", body.Code)
	}
}

func TestAdminReachesAdminRoute(t *testing.T) {
	srv := newAuthServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/admin", nil)
	req.Header.Set("Authorization", "Bearer admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

func TestHealthStaysPublic(t *testing.T) {
	srv := newAuthServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}
