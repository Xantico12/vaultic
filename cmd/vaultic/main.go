package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Xantico12/vaultic/internal/protocol"
)

const serverAddr = "127.0.0.1:7700"

func main() {
    args := os.Args[1:]

    if len(args) == 0 {
        runREPL()
        return
    }

    if err := runOneShot(args); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
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

// runOneShot connects, sends one command built from the CLI args, prints the
// response, and exits.
func runOneShot(args []string) error {
    client, err := protocol.Dial(serverAddr)
    if err != nil {
        return err
    }
    defer client.Close()

    cmd := strings.ToUpper(args[0])
    line := strings.Join(args, " ")

    if err := client.Send(line); err != nil {
        return err
    }

    if cmd == "LIST" {
        values, err := client.ReadUntilEnd()
        if err != nil {
            return err
        }
        for _, v := range values {
            fmt.Println(v)
        }
        return nil
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
        fmt.Fprintln(os.Stderr, resp)
        os.Exit(1)
    default:
        fmt.Println(resp)
    }
    return nil
}