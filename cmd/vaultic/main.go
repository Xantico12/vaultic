package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Xantico12/vaultic/internal/dotenv"
	"github.com/Xantico12/vaultic/internal/protocol"
)

const serverAddr = "127.0.0.1:7700"

func main() {
    if len(os.Args) < 2 {
        runREPL()
        return
    }

    sub := os.Args[1]
    args := os.Args[2:]

    var err error
    switch sub {
    case "set":
        err = cmdSet(args)
    case "get":
        err = cmdGet(args)
    case "delete":
        err = cmdDelete(args)
    case "list":
        err = cmdList(args)
    case "export":
        err = cmdExport(args)
    case "import":
        err = cmdImport(args)
    case "help", "-h", "--help":
        printUsage()
        return
    default:
        fmt.Fprintln(os.Stderr, "unknown subcommand:", sub)
        printUsage()
        os.Exit(1)
    }

    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func printUsage() {
    fmt.Fprintln(os.Stderr, `vaultic — encrypted secrets manager

Usage:
  vaultic                              start REPL
  vaultic set <key> <value>            store a secret
  vaultic get <key>                    retrieve a secret
  vaultic delete <key>                 remove a secret
  vaultic list [<prefix>]              list keys, optionally filtered
  vaultic export <namespace> [flags]   dump a namespace
    --format env|json                  default: env
  vaultic import <file> [flags]        load secrets from a file
    --namespace <ns>                   prefix imported keys with namespace:
    --force                            overwrite existing keys`)
}

// --- subcommand implementations ---
func cmdSet(args []string) error {
    if len(args) != 2 {
        return fmt.Errorf("usage: vaultic set <key> <value>")
    }
    return sendOneShot("SET " + args[0] + " " + args[1])
}

func cmdGet(args []string) error {
    if len(args) != 1 {
        return fmt.Errorf("usage: vaultic get <key>")
    }
    return sendOneShot("GET " + args[0])
}

func cmdDelete(args []string) error {
    if len(args) != 1 {
        return fmt.Errorf("usage: vaultic delete <key>")
    }
    return sendOneShot("DELETE " + args[0])
}

func cmdList(args []string) error {
    line := "LIST"
    if len(args) == 1 {
        line += " " + args[0]
    } else if len(args) > 1 {
        return fmt.Errorf("usage: vaultic list [<prefix>]")
    }

    client, err := protocol.Dial(serverAddr)
    if err != nil {
        return err
    }
    defer client.Close()

    if err := client.Send(line); err != nil {
        return err
    }
    values, err := client.ReadUntilEnd()
    if err != nil {
        return err
    }
    for _, v := range values {
        fmt.Println(v)
    }
    return nil
}

func cmdExport(args []string) error {
    fs := flag.NewFlagSet("export", flag.ExitOnError)
    format := fs.String("format", "env", "output format: env | json")
    fs.Parse(args)

    rest := fs.Args()
    if len(rest) != 1 {
        return fmt.Errorf("usage: vaultic export <namespace> [--format env|json]")
    }
    namespace := rest[0]

    // Fetch all keys with the namespace prefix.
    client, err := protocol.Dial(serverAddr)
    if err != nil {
        return err
    }
    defer client.Close()

    if err := client.Send("LIST " + namespace + ":"); err != nil {
        return err
    }
    rows, err := client.ReadUntilEnd()
    if err != nil {
        return err
    }

    // Strip the namespace prefix and uppercase the key part.
    // "openclaw:telegram_token=abc" -> "TELEGRAM_TOKEN" -> "abc"
    out := make(map[string]string, len(rows))
    prefix := namespace + ":"
    for _, row := range rows {
        eq := strings.IndexByte(row, '=')
        if eq < 0 {
            return fmt.Errorf("malformed LIST row: %q", row)
        }
        fullKey := row[:eq]
        value := row[eq+1:]
        if !strings.HasPrefix(fullKey, prefix) {
            // shouldn't happen — server filters — but defensive
            continue
        }
        bareKey := strings.ToUpper(strings.TrimPrefix(fullKey, prefix))
        out[bareKey] = value
    }

    if len(out) == 0 {
        return fmt.Errorf("no keys found for namespace %q", namespace)
    }

    switch *format {
    case "env":
        fmt.Print(dotenv.Encode(out))
    case "json":
        // Sort keys for deterministic output.
        keys := make([]string, 0, len(out))
        for k := range out {
            keys = append(keys, k)
        }
        sort.Strings(keys)

        ordered := make(map[string]string, len(out))
        for _, k := range keys {
            ordered[k] = out[k]
        }
        b, err := json.MarshalIndent(ordered, "", "  ")
        if err != nil {
            return err
        }
        fmt.Println(string(b))
    default:
        return fmt.Errorf("unknown format %q (want env or json)", *format)
    }

    return nil
}

func cmdImport(args []string) error {
    fs := flag.NewFlagSet("import", flag.ExitOnError)
    namespace := fs.String("namespace", "", "namespace prefix for imported keys (required)")
    force := fs.Bool("force", false, "overwrite existing keys")
    fs.Parse(args)

    rest := fs.Args()
    if len(rest) != 1 {
        return fmt.Errorf("usage: vaultic import <file> --namespace <ns> [--force]")
    }
    if *namespace == "" {
        return fmt.Errorf("--namespace is required")
    }
    file := rest[0]

    f, err := os.Open(file)
    if err != nil {
        return err
    }
    defer f.Close()

    parsed, err := dotenv.Decode(f)
    if err != nil {
        return fmt.Errorf("parse %s: %w", file, err)
    }
    if len(parsed) == 0 {
        return fmt.Errorf("no keys to import from %s", file)
    }

    // Build the namespaced key map: TELEGRAM_TOKEN -> openclaw:telegram_token
    target := make(map[string]string, len(parsed))
    for k, v := range parsed {
        target[*namespace+":"+strings.ToLower(k)] = v
    }

	// Reject newline-containing values until the protocol supports them.
	for k, v := range target {
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("value for %s contains newline — not supported by current protocol", k)
		}
	}

    client, err := protocol.Dial(serverAddr)
    if err != nil {
        return err
    }
    defer client.Close()

    // First pass: check for collisions unless --force.
    if !*force {
        var collisions []string
        for k := range target {
            if err := client.Send("GET " + k); err != nil {
                return err
            }
            resp, err := client.ReadLine()
            if err != nil {
                return err
            }
            if strings.HasPrefix(resp, "VALUE ") {
                collisions = append(collisions, k)
            }
        }
        if len(collisions) > 0 {
            sort.Strings(collisions)
            return fmt.Errorf("%d key(s) already exist (use --force to overwrite):\n  %s",
                len(collisions), strings.Join(collisions, "\n  "))
        }
    }

    // Second pass: write everything.
    var overwritten int
    for k, v := range target {
        if *force {
            // Track whether we're overwriting for the summary.
            if err := client.Send("GET " + k); err != nil {
                return err
            }
            resp, _ := client.ReadLine()
            if strings.HasPrefix(resp, "VALUE ") {
                overwritten++
            }
        }

        if err := client.Send("SET " + k + " " + v); err != nil {
            return err
        }
        resp, err := client.ReadLine()
        if err != nil {
            return err
        }
        if !strings.HasPrefix(resp, "OK") {
            return fmt.Errorf("server rejected SET %s: %s", k, resp)
        }
    }

    if overwritten > 0 {
        fmt.Printf("imported %d keys into namespace %s (%d overwritten)\n",
            len(target), *namespace, overwritten)
    } else {
        fmt.Printf("imported %d keys into namespace %s\n", len(target), *namespace)
    }
    return nil
}

// sendOneShot connects, sends one command, prints the single-line response.
func sendOneShot(line string) error {
    client, err := protocol.Dial(serverAddr)
    if err != nil {
        return err
    }
    defer client.Close()

    if err := client.Send(line); err != nil {
        return err
    }
    resp, err := client.ReadLine()
    if err != nil {
        return err
    }

    switch {
    case resp == "OK":
        fmt.Println("OK")
    case strings.HasPrefix(resp, "VALUE "):
        fmt.Println(strings.TrimPrefix(resp, "VALUE "))
    case strings.HasPrefix(resp, "ERR"):
        return fmt.Errorf("%s", resp)
    default:
        fmt.Println(resp)
    }
    return nil
}


// runREPL opens one persistent connection and loops reading commands from
// stdin, sending them to the server, and printing the response.
func runREPL() {
    client, err := protocol.Dial(serverAddr)
    if err != nil {
        fmt.Fprintln(os.Stderr, "fatal:", err)
        os.Exit(1)
    }
    defer client.Close()

    scanner := bufio.NewScanner(os.Stdin)
    fmt.Println("vaultic CLI — connected to", serverAddr)
    fmt.Println("commands: SET <key> <value> | GET <key> | DELETE <key> | LIST | EXIT")

    for {
        fmt.Print("vaultic> ")
        if !scanner.Scan() {
            return
        }
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        cmd := strings.ToUpper(strings.SplitN(line, " ", 2)[0])

        if cmd == "EXIT" || cmd == "QUIT" {
            client.Send("QUIT")
            client.ReadLine() // BYE
            return
        }

        if err := client.Send(line); err != nil {
            fmt.Fprintln(os.Stderr, "send:", err)
            return
        }

        if cmd == "LIST" {
            values, err := client.ReadUntilEnd()
            if err != nil {
                fmt.Fprintln(os.Stderr, "read:", err)
                return
            }
            for _, v := range values {
                fmt.Println(v)
            }
            continue
        }

        resp, err := client.ReadLine()
        if err != nil {
            fmt.Fprintln(os.Stderr, "read:", err)
            return
        }
        fmt.Println(resp)
    }
}