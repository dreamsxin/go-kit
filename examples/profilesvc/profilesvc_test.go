package profilesvc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kitlog "github.com/dreamsxin/go-kit/log"
)

// ── Service unit tests ────────────────────────────────────────────────────────

func TestInmemService_PostAndGet(t *testing.T) {
	svc := NewInmemService()
	p := Profile{ID: "1", Name: "Alice"}

	if err := svc.PostProfile(context.Background(), p); err != nil {
		t.Fatalf("PostProfile: %v", err)
	}

	got, err := svc.GetProfile(context.Background(), "1")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("Name: got %q, want %q", got.Name, "Alice")
	}
}

func TestInmemService_PostDuplicate(t *testing.T) {
	svc := NewInmemService()
	p := Profile{ID: "1", Name: "Alice"}

	svc.PostProfile(context.Background(), p) //nolint:errcheck
	err := svc.PostProfile(context.Background(), p)
	if err != ErrAlreadyExists {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestInmemService_GetNotFound(t *testing.T) {
	svc := NewInmemService()
	_, err := svc.GetProfile(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestInmemService_PutProfile(t *testing.T) {
	svc := NewInmemService()
	svc.PostProfile(context.Background(), Profile{ID: "1", Name: "Alice"}) //nolint:errcheck

	updated := Profile{ID: "1", Name: "Alice Updated"}
	if err := svc.PutProfile(context.Background(), "1", updated); err != nil {
		t.Fatalf("PutProfile: %v", err)
	}

	got, _ := svc.GetProfile(context.Background(), "1")
	if got.Name != "Alice Updated" {
		t.Errorf("Name: got %q, want %q", got.Name, "Alice Updated")
	}
}

func TestInmemService_PutInconsistentIDs(t *testing.T) {
	svc := NewInmemService()
	err := svc.PutProfile(context.Background(), "1", Profile{ID: "2", Name: "Bob"})
	if err != ErrInconsistentIDs {
		t.Errorf("expected ErrInconsistentIDs, got %v", err)
	}
}

func TestInmemService_PutCreatesNew(t *testing.T) {
	svc := NewInmemService()
	// PUT on non-existent ID should create
	if err := svc.PutProfile(context.Background(), "new", Profile{ID: "new", Name: "New"}); err != nil {
		t.Fatalf("PutProfile: %v", err)
	}
	got, err := svc.GetProfile(context.Background(), "new")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Name != "New" {
		t.Errorf("Name: got %q, want %q", got.Name, "New")
	}
}

// ── Endpoint unit tests ───────────────────────────────────────────────────────

func TestEndpoints_PostAndGet(t *testing.T) {
	svc := NewInmemService()
	eps := MakeServerEndpoints(svc)

	// Post
	_, err := eps.PostProfileEndpoint(context.Background(), postProfileRequest{
		Profile: Profile{ID: "e1", Name: "Endpoint Test"},
	})
	if err != nil {
		t.Fatalf("PostProfileEndpoint: %v", err)
	}

	// Get
	resp, err := eps.GetProfileEndpoint(context.Background(), getProfileRequest{ID: "e1"})
	if err != nil {
		t.Fatalf("GetProfileEndpoint: %v", err)
	}
	gr := resp.(getProfileResponse)
	if gr.Err != nil {
		t.Fatalf("response error: %v", gr.Err)
	}
	if gr.Profile.Name != "Endpoint Test" {
		t.Errorf("Name: got %q, want %q", gr.Profile.Name, "Endpoint Test")
	}
}

func TestEndpoints_GetNotFound(t *testing.T) {
	svc := NewInmemService()
	eps := MakeServerEndpoints(svc)

	resp, err := eps.GetProfileEndpoint(context.Background(), getProfileRequest{ID: "missing"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	gr := resp.(getProfileResponse)
	if gr.Err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", gr.Err)
	}
}

// ── HTTP integration tests ────────────────────────────────────────────────────

func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := NewInmemService()
	handler := MakeHTTPHandler(svc, kitlog.NewNopLogger())
	return httptest.NewServer(handler)
}

func TestHTTP_PostProfile(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	body, _ := json.Marshal(Profile{ID: "h1", Name: "HTTP Test"})
	resp, err := http.Post(srv.URL+"/profiles/", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /profiles/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestHTTP_GetProfile(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	// create first
	body, _ := json.Marshal(Profile{ID: "h2", Name: "Get Test"})
	http.Post(srv.URL+"/profiles/", "application/json", bytes.NewReader(body)) //nolint:errcheck

	resp, err := http.Get(srv.URL + "/profiles/h2")
	if err != nil {
		t.Fatalf("GET /profiles/h2: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result getProfileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Profile.Name != "Get Test" {
		t.Errorf("Name: got %q, want %q", result.Profile.Name, "Get Test")
	}
}

func TestHTTP_GetProfile_NotFound(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/profiles/nonexistent")
	if err != nil {
		t.Fatalf("GET /profiles/nonexistent: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHTTP_PutProfile(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	// create
	body, _ := json.Marshal(Profile{ID: "h3", Name: "Original"})
	http.Post(srv.URL+"/profiles/", "application/json", bytes.NewReader(body)) //nolint:errcheck

	// update
	updated, _ := json.Marshal(Profile{ID: "h3", Name: "Updated"})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/profiles/h3", bytes.NewReader(updated))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /profiles/h3: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// verify
	getResp, _ := http.Get(srv.URL + "/profiles/h3")
	defer getResp.Body.Close()
	var result getProfileResponse
	json.NewDecoder(getResp.Body).Decode(&result) //nolint:errcheck
	if result.Profile.Name != "Updated" {
		t.Errorf("Name after PUT: got %q, want %q", result.Profile.Name, "Updated")
	}
}

func TestHTTP_PostDuplicate_Returns400(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	body, _ := json.Marshal(Profile{ID: "dup", Name: "Dup"})
	http.Post(srv.URL+"/profiles/", "application/json", bytes.NewReader(body)) //nolint:errcheck

	resp, err := http.Post(srv.URL+"/profiles/", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("second POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// ── Endpoints as Service (client-side) ───────────────────────────────────────

func TestEndpoints_ImplementsService(t *testing.T) {
	svc := NewInmemService()
	eps := MakeServerEndpoints(svc)

	// Endpoints implements Service interface
	var _ Service = eps

	if err := eps.PostProfile(context.Background(), Profile{ID: "s1", Name: "Service"}); err != nil {
		t.Fatalf("PostProfile via Endpoints: %v", err)
	}
	p, err := eps.GetProfile(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetProfile via Endpoints: %v", err)
	}
	if p.Name != "Service" {
		t.Errorf("Name: got %q, want %q", p.Name, "Service")
	}
}
