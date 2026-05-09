package protocol

import (
	"bufio"
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Xantico12/vaultic/pkg/vault"
)

// startTestServer spins up a Server on a random local port, backed by a
// fresh per-test WAL. Returns the address and a cleanup function.
func startTestServer(t *testing.T) (addr string, cleanup func()) {
    t.Helper()

    walPath := filepath.Join(t.TempDir(), "vaultic.wal")
    store, err := vault.NewStore(walPath, []byte("test-password"))
    if err != nil {
        t.Fatalf("NewStore: %v", err)
    }

    // Bind to :0 so the OS picks a free port for us.
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("Listen: %v", err)
    }
    addr = listener.Addr().String()
    listener.Close() // re-bound by Serve

    ctx, cancel := context.WithCancel(context.Background())
    server := NewServer(store)

    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        _ = server.Serve(ctx, addr, nil)
    }()

    // Tiny pause to let the server bind. Better: poll-connect.
    waitUntilListening(t, addr)

    cleanup = func() {
        cancel()
        wg.Wait()
        store.Close()
    }
    return addr, cleanup
}

func waitUntilListening(t *testing.T, addr string) {
    t.Helper()
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        conn, err := net.Dial("tcp", addr)
        if err == nil {
            conn.Close()
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatalf("server didn't start listening on %s within 2s", addr)
}

// sendCommand connects, sends one line, reads one line back.
func sendCommand(t *testing.T, addr, cmd string) string {
    t.Helper()
    conn, err := net.Dial("tcp", addr)
    if err != nil {
        t.Fatalf("Dial: %v", err)
    }
    defer conn.Close()

    if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
        t.Fatalf("Write: %v", err)
    }
    line, err := bufio.NewReader(conn).ReadString('\n')
    if err != nil {
        t.Fatalf("ReadString: %v", err)
    }
    return strings.TrimRight(line, "\r\n")
}

func TestServerSetGetRoundtrip(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    if got := sendCommand(t, addr, "SET foo bar"); got != "OK" {
        t.Errorf("SET = %q, want OK", got)
    }
    if got := sendCommand(t, addr, "GET foo"); got != "VALUE bar" {
        t.Errorf("GET = %q, want VALUE bar", got)
    }
}

func TestServerGetMissingKey(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    if got := sendCommand(t, addr, "GET nope"); got != "ERR not found" {
        t.Errorf("GET = %q, want ERR not found", got)
    }
}

func TestServerDelete(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    sendCommand(t, addr, "SET k v")
    if got := sendCommand(t, addr, "DELETE k"); got != "OK" {
        t.Errorf("DELETE = %q, want OK", got)
    }
    if got := sendCommand(t, addr, "GET k"); got != "ERR not found" {
        t.Errorf("GET after DELETE = %q, want ERR not found", got)
    }
}

func TestServerListFraming(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    sendCommand(t, addr, "SET a 1")
    sendCommand(t, addr, "SET b 2")

    conn, err := net.Dial("tcp", addr)
    if err != nil {
        t.Fatalf("Dial: %v", err)
    }
    defer conn.Close()

    conn.Write([]byte("LIST\n"))

    reader := bufio.NewReader(conn)
    var lines []string
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            t.Fatalf("ReadString: %v", err)
        }
        line = strings.TrimRight(line, "\r\n")
        if line == "END" {
            break
        }
        lines = append(lines, line)
    }

    if len(lines) != 2 {
        t.Errorf("got %d VALUE lines, want 2: %v", len(lines), lines)
    }
    for _, line := range lines {
        if !strings.HasPrefix(line, "VALUE ") {
            t.Errorf("expected VALUE prefix, got %q", line)
        }
    }
}

func TestServerConcurrentClients(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    var wg sync.WaitGroup
    for i := 0; i < 20; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            key := "key" + string(rune('A'+n%26))
            sendCommand(t, addr, "SET "+key+" value")
            got := sendCommand(t, addr, "GET "+key)
            if got != "VALUE value" {
                t.Errorf("client %d: GET = %q", n, got)
            }
        }(i)
    }
    wg.Wait()
}

func TestServerGracefulShutdown(t *testing.T) {
    walPath := filepath.Join(t.TempDir(), "vaultic.wal")
    store, _ := vault.NewStore(walPath, []byte("p"))
    defer store.Close()

    listener, _ := net.Listen("tcp", "127.0.0.1:0")
    addr := listener.Addr().String()
    listener.Close()

    ctx, cancel := context.WithCancel(context.Background())
    server := NewServer(store)

    done := make(chan struct{})
    go func() {
        server.Serve(ctx, addr, nil)
        close(done)
    }()
    waitUntilListening(t, addr)

    // Open a connection, leave it idle, then cancel.
    conn, _ := net.Dial("tcp", addr)
    defer conn.Close()

    cancel()

    select {
    case <-done:
        // good — Serve returned
    case <-time.After(2 * time.Second):
        t.Fatal("Serve didn't return within 2s of cancel()")
    }
}

func TestServerListPrefix(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    sendCommand(t, addr, "SET openclaw:a 1")
    sendCommand(t, addr, "SET openclaw:b 2")
    sendCommand(t, addr, "SET adpulse:c 3")

    conn, err := net.Dial("tcp", addr)
    if err != nil {
        t.Fatalf("Dial: %v", err)
    }
    defer conn.Close()

    conn.Write([]byte("LIST openclaw:\n"))

    reader := bufio.NewReader(conn)
    var lines []string
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            t.Fatalf("ReadString: %v", err)
        }
        line = strings.TrimRight(line, "\r\n")
        if line == "END" {
            break
        }
        lines = append(lines, line)
    }

    if len(lines) != 2 {
        t.Errorf("got %d lines for prefix openclaw:, want 2: %v", len(lines), lines)
    }
    for _, line := range lines {
        if !strings.Contains(line, "openclaw:") {
            t.Errorf("non-namespace line slipped through filter: %q", line)
        }
    }
}