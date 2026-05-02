package protocol

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Xantico12/vaultic/internal/tlsutil"
	"github.com/Xantico12/vaultic/pkg/vault"
)

// startTestServerTLS spins up a Server on a random local port using a fresh
// self-signed cert. Returns the addr, a *tls.Config the client should use,
// and a cleanup function.
func startTestServerTLS(t *testing.T) (string, *tls.Config, func()) {
    t.Helper()

    dir := t.TempDir()
    walPath := filepath.Join(dir, "vaultic.wal")
    certPath := filepath.Join(dir, "vaultic.cert")
    keyPath := filepath.Join(dir, "vaultic.key")

    store, err := vault.NewStore(walPath, []byte("test-password"))
    if err != nil {
        t.Fatalf("NewStore: %v", err)
    }

    serverCert, err := tlsutil.LoadOrGenerateCert(certPath, keyPath)
    if err != nil {
        t.Fatalf("LoadOrGenerateCert: %v", err)
    }

    serverTLSConfig := &tls.Config{
        Certificates: []tls.Certificate{serverCert},
        MinVersion:   tls.VersionTLS13,
    }

    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("Listen: %v", err)
    }
    addr := listener.Addr().String()
    listener.Close()

    ctx, cancel := context.WithCancel(context.Background())
    server := NewServer(store)

    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        _ = server.Serve(ctx, addr, serverTLSConfig)
    }()

    // Wait for listener via a TLS dial — proves both bind and handshake work.
    clientTLSConfig := buildClientTLSConfig(t, certPath)
    waitUntilTLSListening(t, addr, clientTLSConfig)

    cleanup := func() {
        cancel()
        wg.Wait()
        store.Close()
    }
    return addr, clientTLSConfig, cleanup
}

func buildClientTLSConfig(t *testing.T, certPath string) *tls.Config {
    t.Helper()
    raw, err := os.ReadFile(certPath)
    if err != nil {
        t.Fatalf("ReadFile %s: %v", certPath, err)
    }
    block, _ := pem.Decode(raw)
    if block == nil {
        t.Fatalf("pem.Decode returned nil for %s", certPath)
    }
    cert, err := x509.ParseCertificate(block.Bytes)
    if err != nil {
        t.Fatalf("ParseCertificate: %v", err)
    }
    pool := x509.NewCertPool()
    pool.AddCert(cert)
    return &tls.Config{
        RootCAs:    pool,
        ServerName: "localhost",
        MinVersion: tls.VersionTLS13,
    }
}

func waitUntilTLSListening(t *testing.T, addr string, cfg *tls.Config) {
    t.Helper()
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        conn, err := tls.Dial("tcp", addr, cfg)
        if err == nil {
            conn.Close()
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatalf("TLS server didn't start listening on %s within 2s", addr)
}

func TestServerTLSRoundtrip(t *testing.T) {
    addr, clientCfg, cleanup := startTestServerTLS(t)
    defer cleanup()

    client, err := Dial(addr, clientCfg)
    if err != nil {
        t.Fatalf("Dial: %v", err)
    }
    defer client.Close()

    if err := client.Send("SET foo bar"); err != nil {
        t.Fatalf("Send SET: %v", err)
    }
    if got, _ := client.ReadLine(); got != "OK" {
        t.Errorf("SET response = %q, want OK", got)
    }

    if err := client.Send("GET foo"); err != nil {
        t.Fatalf("Send GET: %v", err)
    }
    if got, _ := client.ReadLine(); got != "VALUE bar" {
        t.Errorf("GET response = %q, want VALUE bar", got)
    }
}

func TestServerTLSRejectsPlainClient(t *testing.T) {
    addr, _, cleanup := startTestServerTLS(t)
    defer cleanup()

    // Plain (non-TLS) Dial against a TLS-only server should fail to round-trip.
    client, err := Dial(addr, nil)
    if err != nil {
        return // some Go versions error during Dial — also fine
    }
    defer client.Close()

    err = client.Send("LIST")
    if err == nil {
        if _, err = client.ReadLine(); err == nil {
            t.Error("expected plain client to fail talking to TLS server, got nil error")
        }
    }
}

func TestClientRejectsUntrustedServer(t *testing.T) {
    addr, _, cleanup := startTestServerTLS(t)
    defer cleanup()

    // Build a client config with an EMPTY cert pool — server's cert will not be trusted.
    untrustingCfg := &tls.Config{
        RootCAs:    x509.NewCertPool(),
        ServerName: "localhost",
        MinVersion: tls.VersionTLS13,
    }

    if _, err := Dial(addr, untrustingCfg); err == nil {
        t.Error("Dial with empty trust pool should have failed, got nil")
    }
}