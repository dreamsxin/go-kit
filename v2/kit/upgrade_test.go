package kit_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// TestHijackedConnectionOutlivesShutdown proves the limit of the shutdown
// sequence. http.Server.Shutdown does not track a hijacked connection and closing
// the listener does not close one, so an upgraded connection — a WebSocket, or
// anything else that took the socket — is outside the grace period. Milestone 11's
// promise still holds, because a hijacked connection is one the server no longer
// owns; what would be wrong is letting a reader believe otherwise.
func TestHijackedConnectionOutlivesShutdown(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")

	hijacked := make(chan net.Conn, 1)
	component.HandleFunc("GET /upgrade", func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement http.Hijacker")
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: test\r\nConnection: Upgrade\r\n\r\n")
		_ = buffered.Flush()
		hijacked <- conn
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	conn, err := net.Dial("tcp", component.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "GET /upgrade HTTP/1.1\r\nHost: %s\r\n\r\n", component.Addr()); err != nil {
		t.Fatalf("write request: %v", err)
	}
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("status = %q, want 101", status)
	}
	drainHeaders(t, reader)

	var serverSide net.Conn
	select {
	case serverSide = <-hijacked:
	case <-time.After(2 * time.Second):
		t.Fatal("the handler never hijacked the connection")
	}
	defer serverSide.Close()

	// Shutdown returns promptly and reports success: there is nothing it is
	// waiting for, because it is not tracking this connection.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := component.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown = %v, want nil: a hijacked connection is not waited for", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown took %s, want it not to wait on the hijacked connection", elapsed)
	}

	// And the connection still works, which is the fact worth declaring: ending
	// it is the upgraded handler's job, not the server's.
	if _, err := serverSide.Write([]byte("still here\n")); err != nil {
		t.Fatalf("write on the hijacked connection after shutdown: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read after shutdown: %v", err)
	}
	if strings.TrimSpace(line) != "still here" {
		t.Fatalf("read %q after shutdown", line)
	}

	// The listener is gone even though the upgraded connection is not.
	if _, err := net.DialTimeout("tcp", component.Addr(), 500*time.Millisecond); err == nil {
		t.Error("the listener still accepts connections after shutdown")
	}
}

// TestPlaintextListenerSpeaksHTTP11 is the other half of the statement: without
// TLS there is no ALPN and h2c is not offered, so the plaintext port stays HTTP/1.1
// — which is what keeps hijack-based upgrades working there.
func TestPlaintextListenerSpeaksHTTP11(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	component.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 1 {
			t.Errorf("server saw HTTP/%d, want HTTP/1.x", r.ProtoMajor)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = component.Shutdown(context.Background()) }()

	resp, err := http.Get("http://" + component.Addr() + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()
	if resp.Proto != "HTTP/1.1" {
		t.Fatalf("proto = %s, want HTTP/1.1: h2c is not offered", resp.Proto)
	}
}

// drainHeaders consumes the response headers up to and including the blank line
// that ends them, so what is read next is whatever the upgraded protocol sends.
func drainHeaders(t *testing.T, reader *bufio.Reader) {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read header: %v", err)
		}
		if strings.TrimSpace(line) == "" {
			return
		}
	}
}

// TestUpgradedHandlerIsToldTheProcessIsStopping proves the seam that makes the
// limit workable: an upgraded handler gets the stopping signal like any other, so
// it can end its own connection while the process is still draining.
func TestUpgradedHandlerIsToldTheProcessIsStopping(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")

	told := make(chan bool, 1)
	component.HandleFunc("GET /upgrade", func(w http.ResponseWriter, r *http.Request) {
		hijacker, _ := w.(http.Hijacker)
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer conn.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\n\r\n")
		_ = buffered.Flush()

		select {
		case <-kit.Stopping(r.Context()):
			told <- true
		case <-time.After(3 * time.Second):
			told <- false
		}
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	conn, err := net.Dial("tcp", component.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "GET /upgrade HTTP/1.1\r\nHost: %s\r\n\r\n", component.Addr()); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if _, err := bufio.NewReader(conn).ReadString('\n'); err != nil {
		t.Fatalf("read status: %v", err)
	}

	host := kit.MustNewHost(kit.WithLifecycle(component))
	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	select {
	case stopped := <-told:
		if !stopped {
			t.Fatal("the upgraded handler was never told the process is stopping")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the upgraded handler did not react to the announcement")
	}
	_ = component.Shutdown(context.Background())
}

// TestSSEStillStreamsOverHTTP2 proves the protocol change TLS brings does not
// break streaming: over h2 there is no chunked framing and flushing goes through
// the stream layer instead, so this is the case a reader would reasonably worry
// about after learning that turning on TLS also turns on HTTP/2.
func TestSSEStillStreamsOverHTTP2(t *testing.T) {
	certPath, keyPath, pool := selfSignedCert(t)
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithTLS(certPath, keyPath))
	release := make(chan struct{})
	component.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not implement http.Flusher over h2")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 2; i++ {
			_, _ = fmt.Fprintf(w, "data: event-%d\n\n", i)
			flusher.Flush()
		}
		<-release
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		close(release)
		_ = component.Shutdown(context.Background())
	}()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}}
	resp, err := client.Get("https://" + component.Addr() + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("proto = %s, want HTTP/2.0", resp.Proto)
	}

	reader := bufio.NewReader(resp.Body)
	for i := 0; i < 2; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if want := fmt.Sprintf("data: event-%d", i); strings.TrimSpace(line) != want {
			t.Fatalf("event %d = %q, want %q", i, strings.TrimSpace(line), want)
		}
		if _, err := reader.ReadString('\n'); err != nil { // the blank separator
			t.Fatalf("read separator %d: %v", i, err)
		}
	}
}
