package protocol

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

// Client is a single connection to a vaultic server.
// Not safe for concurrent use - one Client per goroutine.
type Client struct {
	conn	net.Conn
	reader	*bufio.Reader
	writer	*bufio.Writer
}

// Dial opens a connection to a vaultic server at addr.
func Dial(addr string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return &Client{
		conn:	conn,
		reader:	bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}, nil
}

// Close terminates the connection.
func (c *Client) Close() error {
    return c.conn.Close()
}

// Send writes a command and flushes it to the server.
func (c *Client) Send(line string) error {
    if _, err := c.writer.WriteString(line + "\n"); err != nil {
        return err
    }
    return c.writer.Flush()
}

// ReadLine reads one response line, trimming the trailing newline.
func (c *Client) ReadLine() (string, error) {
    line, err := c.reader.ReadString('\n')
    if err != nil {
        return "", err
    }
    return strings.TrimRight(line, "\r\n"), nil
}

// ReadUntilEnd reads multi-line responses (LIST) until it sees an END line.
// Returns all VALUE lines (with the "VALUE " prefix stripped).
func (c *Client) ReadUntilEnd() ([]string, error) {
    var values []string
    for {
        line, err := c.ReadLine()
        if err != nil {
            return nil, err
        }
        if line == "END" {
            return values, nil
        }
        if strings.HasPrefix(line, "VALUE ") {
            values = append(values, strings.TrimPrefix(line, "VALUE "))
            continue
        }
        if strings.HasPrefix(line, "ERR") {
            return nil, fmt.Errorf("server error: %s", line)
        }
        return nil, fmt.Errorf("unexpected response: %q", line)
    }
}
