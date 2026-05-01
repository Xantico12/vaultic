package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	store, err := NewStore("vaultic.wal")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
	defer store.Close()
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("vaultic v0.1")
	fmt.Println("commands: SET <key> <value> | GET <key> | DELETE <key> | LIST | EXIT")
	fmt.Println()

	for {
		fmt.Print("vaultic> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		cmd := strings.ToUpper(parts[0])

		switch cmd {
		case "SET":
			if len(parts) < 3 {
				fmt.Println("usage: SET <key> <value>")
				continue
			}
			if err := store.Set(parts[1], parts[2]); err != nil {
				fmt.Println("ERR:", err)
				continue
			}
			fmt.Println("OK")

		case "GET":
			if len(parts) < 2 {
				fmt.Println("usage: GET <key>")
				continue
			}
			val, exists := store.Get(parts[1])
			if !exists {
				fmt.Println("ERR: Key not found")
			} else {
				fmt.Println(val)
			}

		case "DELETE":
			if len(parts) < 2 {
				fmt.Println("usage: DELETE <key>")
				continue
			}
			if _, exists := store.Get(parts[1]); !exists {
				fmt.Println("ERR: Key not found")
				continue
			}
			if err := store.Delete(parts[1]); err != nil {
				fmt.Println("ERR:", err)
				continue
			}
			fmt.Println("OK")

		case "LIST":
			items := store.List()
			if len(items) == 0 {
				fmt.Println("(empty)")
			} else {
				for k, v := range items {
					fmt.Printf(" %s = %s\n", k, v)
				}
			}

		case "EXIT":
			fmt.Println("bye")
			return
		
		default:
			fmt.Println("unknown command")
		}
	}
}