package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/Xantico12/vaultic/internal/protocol"
	"github.com/Xantico12/vaultic/pkg/vault"
)

const (
    walPath = "vaultic.wal"
    addr    = "127.0.0.1:7700"
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

    // Set up cancellation: Ctrl-C / SIGTERM -> ctx.Done()
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    server := protocol.NewServer(store)
    if err := server.Serve(ctx, addr); err != nil {
        fmt.Fprintln(os.Stderr, "server error:", err)
        os.Exit(1)
    }

    fmt.Fprintln(os.Stderr, "shutdown complete")
}