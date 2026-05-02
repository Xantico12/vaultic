package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

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

    _ = format     // wired in step 4
    _ = namespace
    return fmt.Errorf("export: not implemented yet")
}

func cmdImport(args []string) error {
    fs := flag.NewFlagSet("import", flag.ExitOnError)
    namespace := fs.String("namespace", "", "namespace prefix for imported keys")
    force := fs.Bool("force", false, "overwrite existing keys")
    fs.Parse(args)

    rest := fs.Args()
    if len(rest) != 1 {
        return fmt.Errorf("usage: vaultic import <file> [--namespace <ns>] [--force]")
    }
    file := rest[0]

    _ = namespace  // wired in step 4
    _ = force
    _ = file
    return fmt.Errorf("import: not implemented yet")
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