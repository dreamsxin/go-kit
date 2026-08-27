package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func newCustomCodecServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := kit.MustNewHTTP(":0")
	decode, encode := server.RawBodyCodec(unmarshalCustom, marshalCustom, contentType)
	svc.Handle("POST /shout", server.NewServer(
		shout, decode, encode,
		server.ServerErrorEncoder(server.TextErrorEncoder(contentType)),
	))
	return httptest.NewServer(svc)
}

func TestCustomFormatRoundTrip(t *testing.T) {
	srv := newCustomCodecServer(t)
	defer srv.Close()

	body := append([]byte{5}, []byte("world")...)
	resp, err := http.Post(srv.URL+"/shout", contentType, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != contentType {
		t.Errorf("Content-Type: got %q", ct)
	}
	data, _ := io.ReadAll(resp.Body)
	// [13]['H','E','L','L','O',',',' ','w','o','r','l','d','!']
	if len(data) < 2 || int(data[0]) != 13 {
		t.Fatalf("unexpected wire bytes: %v", data)
	}
	if string(data[1:]) != "HELLO, world!" {
		t.Errorf("payload: got %q", data[1:])
	}
}

func TestInvalidBodyReturns400InSameFormat(t *testing.T) {
	srv := newCustomCodecServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/shout", contentType, bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != contentType {
		t.Errorf("error Content-Type: got %q, want the custom format", ct)
	}
	data, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(data, []byte("length prefix")) && !bytes.Contains(data, []byte("length")) {
		t.Errorf("error body should carry the public message: %q", data)
	}
}

func TestHealthRouteUnaffected(t *testing.T) {
	srv := newCustomCodecServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status: got %d, want 200", resp.StatusCode)
	}
}
