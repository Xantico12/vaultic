package main

import (
    "context"
    "crypto/tls"
    "errors"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "sort"
    "strings"
    "syscall"
    "time"

    "golang.org/x/term"

    "github.com/Xantico12/vaultic/internal/auth"
    "github.com/Xantico12/vaultic/internal/protocol"
    "github.com/Xantico12/vaultic/internal/tlsutil"
    "github.com/Xantico12/vaultic/pkg/vault"
)

const (
    walPath    = "vaultic.wal"
    certPath   = "vaultic.cert"
    keyPath    = "vaultic.key"
    tokensPath = "vaultic.tokens"
    addr       = "127.0.0.1:7700"
)

func main() {
    if len(os.Args) >= 2 && os.Args[1] == "token" {
        runTokenAdmin(os.Args[2:])
        return
    }
    runServer()
}

func runServer() {
    store, _ := openStore()           // (existing logic — extract into helper)
    defer store.Close()

    cert, err := tlsutil.LoadOrGenerateCert(certPath, keyPath)
    if err != nil {
        fmt.Fprintln(os.Stderr, "fatal: tls cert:", err)
        os.Exit(1)
    }
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

// openStore extracts the password-prompting + NewStore + zero-pw logic so
// runServer and runTokenAdmin both reuse it.
func openStore() (*vault.Store, error) {
    fmt.Fprint(os.Stderr, "Master password: ")
    pw, err := term.ReadPassword(int(os.Stdin.Fd()))
    fmt.Fprintln(os.Stderr)
    if err != nil {
        return nil, err
    }

    store, err := vault.NewStore(walPath, pw)
    for i := range pw {
        pw[i] = 0
    }
    if err != nil {
        if errors.Is(err, vault.ErrInvalidPassword) {
            return nil, fmt.Errorf("invalid password")
        }
        return nil, err
    }
    return store, nil
}

func runTokenAdmin(args []string) {
    if len(args) == 0 {
        fmt.Fprintln(os.Stderr, "usage: vaultic-server token <create|list|revoke> [args]")
        os.Exit(1)
    }

    // Verify master password before allowing token ops.
    store, err := openStore()
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    store.Close() // we don't need the store further; just verified the pw

    reg, err := auth.LoadRegistry(tokensPath)
    if err != nil {
        fmt.Fprintln(os.Stderr, "load tokens:", err)
        os.Exit(1)
    }

    sub := args[0]
    rest := args[1:]

    switch sub {
    case "create":
        err = tokenCreate(reg, rest)
    case "list":
        err = tokenList(reg, rest)
    case "revoke":
        err = tokenRevoke(reg, rest)
    default:
        fmt.Fprintln(os.Stderr, "unknown token subcommand:", sub)
        os.Exit(1)
    }
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func tokenCreate(reg *auth.Registry, args []string) error {
    fs := flag.NewFlagSet("token create", flag.ExitOnError)
    permsRaw := fs.String("perms", "", `permissions, e.g. "openclaw=rw,adpulse=r" (required)`)
    fs.Parse(args)

    if *permsRaw == "" {
        return fmt.Errorf("--perms is required")
    }
    perms, err := parsePerms(*permsRaw)
    if err != nil {
        return err
    }

    raw, record, err := auth.Generate(perms)
    if err != nil {
        return err
    }
    if err := reg.Add(record); err != nil {
        return err
    }

    fmt.Println("ID:   ", record.ID)
    fmt.Println("Token:", raw)
    fmt.Println()
    fmt.Println("Save this token — it will not be shown again.")
    return nil
}

func tokenList(reg *auth.Registry, args []string) error {
    records := reg.List()
    sort.Slice(records, func(i, j int) bool {
        return records[i].CreatedAt.Before(records[j].CreatedAt)
    })

    if len(records) == 0 {
        fmt.Println("(no tokens)")
        return nil
    }

    fmt.Printf("%-20s  %-22s  %s\n", "ID", "CREATED", "PERMISSIONS")
    for _, rec := range records {
        ts := rec.CreatedAt.UTC().Format(time.RFC3339)
        fmt.Printf("%-20s  %-22s  %s\n", rec.ID, ts, formatPerms(rec.Permissions))
    }
    return nil
}

func tokenRevoke(reg *auth.Registry, args []string) error {
    if len(args) != 1 {
        return fmt.Errorf("usage: vaultic-server token revoke <id>")
    }
    if err := reg.Revoke(args[0]); err != nil {
        return err
    }
    fmt.Println("revoked:", args[0])
    return nil
}

// --- perms parsing helpers ---

func parsePerms(s string) (map[string]auth.Permission, error) {
    out := make(map[string]auth.Permission)
    for _, pair := range strings.Split(s, ",") {
        pair = strings.TrimSpace(pair)
        if pair == "" {
            continue
        }
        eq := strings.IndexByte(pair, '=')
        if eq < 0 {
            return nil, fmt.Errorf("invalid perm spec %q (want ns=r|rw|admin)", pair)
        }
        ns := strings.TrimSpace(pair[:eq])
        level := strings.TrimSpace(pair[eq+1:])
        var p auth.Permission
        switch level {
        case "r":
            p = auth.PermRead
        case "rw":
            p = auth.PermReadWrite
        case "admin":
            p = auth.PermAdmin
        default:
            return nil, fmt.Errorf("unknown perm level %q (want r, rw, admin)", level)
        }
        out[ns] = p
    }
    if len(out) == 0 {
        return nil, fmt.Errorf("no permissions specified")
    }
    return out, nil
}

func formatPerms(perms map[string]auth.Permission) string {
    if len(perms) == 0 {
        return "(revoked)"
    }
    keys := make([]string, 0, len(perms))
    for k := range perms {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    parts := make([]string, 0, len(keys))
    for _, k := range keys {
        parts = append(parts, fmt.Sprintf("%s=%s", k, permLevelString(perms[k])))
    }
    return strings.Join(parts, ", ")
}

func permLevelString(p auth.Permission) string {
    switch p {
    case auth.PermRead:
        return "r"
    case auth.PermReadWrite:
        return "rw"
    case auth.PermAdmin:
        return "admin"
    default:
        return "?"
    }
}