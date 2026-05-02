package protocol

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/Xantico12/vaultic/pkg/vault"
)

// Server wraps a vault.Store and serves it over TCP.
type Server struct {
    store *vault.Store
    wg    sync.WaitGroup
}

// NewServer creates a new TCP protocol server backed by the given Store.
func NewServer(store *vault.Store) *Server {
    return &Server{store: store}
}

// Serve listens on addr and handles connections until ctx is cancelled.
// Blocks until the listener closes and all in-flight handlers finish.
func (s *Server) Serve(ctx context.Context, addr string) error {
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("listen on %s: %w", addr, err)
    }

    // When ctx is cancelled, close the listener to unblock Accept below.
    go func() {
        <-ctx.Done()
        listener.Close()
    }()

    fmt.Fprintln(os.Stderr, "vaultic-server listening on", addr)

    for {
        conn, err := listener.Accept()
        if err != nil {
            // Listener closed (probably via ctx). Stop accepting.
            if ctx.Err() != nil {
                break
            }
            // Some other error — log and continue.
            fmt.Fprintln(os.Stderr, "accept error:", err)
            continue
        }

        s.wg.Add(1)
        go func() {
            defer s.wg.Done()
            s.handleConnection(ctx, conn)
        }()
    }

    s.wg.Wait()
    return nil
}

// handleConnection reads commands line-by-line from conn, executes them,
// and writes responses. Returns when the client disconnects or ctx is done.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	writer := bufio.NewWriter(conn)
	defer writer.Flush()

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		s.dispatch(line, writer)
		writer.Flush()
	}
}

// dispatch parses a single line of input, executes the command, and writes
// the response to w. The caller is responsible for flushing.
func (s *Server) dispatch(line string, w *bufio.Writer) {
    parts := strings.SplitN(line, " ", 3)
    cmd := strings.ToUpper(parts[0])

    switch cmd {
    case "SET":
        if len(parts) < 3 {
            fmt.Fprintln(w, "ERR usage: SET <key> <value>")
            return
        }
        if err := s.store.Set(parts[1], parts[2]); err != nil {
            fmt.Fprintln(w, "ERR", err)
            return
        }
        fmt.Fprintln(w, "OK")

    case "GET":
        if len(parts) < 2 {
            fmt.Fprintln(w, "ERR usage: GET <key>")
            return
        }
        val, ok := s.store.Get(parts[1])
        if !ok {
            fmt.Fprintln(w, "ERR not found")
            return
        }
        fmt.Fprintln(w, "VALUE", val)

    case "DELETE":
        if len(parts) < 2 {
            fmt.Fprintln(w, "ERR usage: DELETE <key>")
            return
        }
        if _, ok := s.store.Get(parts[1]); !ok {
            fmt.Fprintln(w, "ERR not found")
            return
        }
        if err := s.store.Delete(parts[1]); err != nil {
            fmt.Fprintln(w, "ERR", err)
            return
        }
        fmt.Fprintln(w, "OK")

    case "LIST":
        var prefix string
        if len(parts) >= 2 {
            prefix = parts[1]
        }
        items := s.store.List()
        for k, v := range items {
            if prefix == "" || strings.HasPrefix(k, prefix) {
                fmt.Fprintf(w, "VALUE %s=%s\n", k, v)
            }
        }
        fmt.Fprintln(w, "END")

    case "QUIT":
        fmt.Fprintln(w, "BYE")

    default:
        fmt.Fprintln(w, "ERR unknown command")
    }
}