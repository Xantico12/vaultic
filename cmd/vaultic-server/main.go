package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/Xantico12/vaultic/internal/protocol"
	"github.com/Xantico12/vaultic/internal/tlsutil"
	"github.com/Xantico12/vaultic/pkg/vault"
)

const (
    walPath  = "vaultic.wal"
    certPath = "vaultic.cert"
    keyPath  = "vaultic.key"
    addr     = "127.0.0.1:7700"
)

func main() {
    fmt.Fprint(os.Stderr, "Master password: ")
    pw, err := term.ReadPassword(int(os.Stdin.Fd()))
    fmt.Fprintln(os.Stderr)
    if err != nil {
        fmt.Fprintln(os.Stderr, "fatal: could not read password:", err)
        os.Exit(1)
    }

    store, err := vault.NewStore(walPath, pw)
    if err != nil {
        if errors.Is(err, vault.ErrInvalidPassword) {
            fmt.Fprintln(os.Stderr, "Invalid password.")
            os.Exit(1)
        }
        fmt.Fprintln(os.Stderr, "fatal:", err)
        os.Exit(1)
    }
    defer store.Close()

    // Zero the password — derived key lives in store.key now.
    for i := range pw {
        pw[i] = 0
    }

    // Load or generate TLS material.
    cert, err := tlsutil.LoadOrGenerateCert(certPath, keyPath)
    if err != nil {
        fmt.Fprintln(os.Stderr, "fatal: tls cert:", err)
        os.Exit(1)
    }

    // Show the fingerprint so the user can verify it from the client side.
    fmt.Fprintln(os.Stderr, "TLS fingerprint:", tlsutil.Fingerprint(cert.Certificate[0]))

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS13,
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    server := protocol.NewServer(store)
    if err := server.Serve(ctx, addr, tlsConfig); err != nil {
        fmt.Fprintln(os.Stderr, "server error:", err)
        os.Exit(1)
    }

    fmt.Fprintln(os.Stderr, "shutdown complete")
}