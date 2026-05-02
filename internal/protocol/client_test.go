package protocol

import (
	"strings"
	"testing"
)

func TestClientSetGet(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    client, err := Dial(addr)
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

func TestClientList(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    client, _ := Dial(addr)
    defer client.Close()

    client.Send("SET a 1")
    client.ReadLine()
    client.Send("SET b 2")
    client.ReadLine()

    if err := client.Send("LIST"); err != nil {
        t.Fatalf("Send LIST: %v", err)
    }

    values, err := client.ReadUntilEnd()
    if err != nil {
        t.Fatalf("ReadUntilEnd: %v", err)
    }
    if len(values) != 2 {
        t.Errorf("got %d values, want 2: %v", len(values), values)
    }

    seen := map[string]bool{}
    for _, v := range values {
        seen[v] = true
    }
    if !seen["a=1"] || !seen["b=2"] {
        t.Errorf("missing entries in LIST: %v", values)
    }
}

func TestClientNotFound(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    client, _ := Dial(addr)
    defer client.Close()

    client.Send("GET missing")
    resp, _ := client.ReadLine()
    if !strings.HasPrefix(resp, "ERR") {
        t.Errorf("expected ERR prefix, got %q", resp)
    }
}

func TestClientPersistentConnection(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    client, _ := Dial(addr)
    defer client.Close()

    // Send 50 commands over the same connection — equivalent to REPL mode.
    for i := 0; i < 50; i++ {
        client.Send("SET k v")
        if got, _ := client.ReadLine(); got != "OK" {
            t.Fatalf("iteration %d: got %q", i, got)
        }
    }
}

func TestClientReadUntilEndOnError(t *testing.T) {
    addr, cleanup := startTestServer(t)
    defer cleanup()

    client, _ := Dial(addr)
    defer client.Close()

    // GET returns a single VALUE/ERR line — using ReadUntilEnd against it
    // should surface the error instead of hanging or returning bad data.
    client.Send("GET missing")
    if _, err := client.ReadUntilEnd(); err == nil {
        t.Error("ReadUntilEnd on ERR response should have errored")
    }
}

func TestClientDialRefused(t *testing.T) {
    // 127.0.0.1:1 is reserved and nothing should be listening.
    if _, err := Dial("127.0.0.1:1"); err == nil {
        t.Error("Dial to dead address should have errored")
    }
}