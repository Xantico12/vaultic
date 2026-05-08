// Package dotenv encodes and decodes the de-facto .env file format used by
// dotenv (Node), python-dotenv, godotenv, and similar libraries.
//
// Supported syntax:
//
//	# comments allowed (full-line)
//	KEY=plain value
//	QUOTED="hello world"
//	ESCAPED="line1\nline2"           // \n, \t, \r, \\, \", \$ inside double quotes
//	SINGLE='no escapes here'
//
// NOT supported (deliberately):
//   - Variable expansion ($HOME, ${HOME})
//   - Command substitution ($(...) or backticks)
//   - Multiline values
//   - Trailing comments on the same line as a value
//
// Rejecting these keeps imports safe — a .env file can never trigger code
// execution or environment lookups.
package dotenv

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Encode renders the map as a .env file. Keys are sorted for deterministic
// output. Values are quoted only if needed (whitespace, special chars).
func Encode(m map[string]string) string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    var b strings.Builder
    for _, k := range keys {
        b.WriteString(k)
        b.WriteByte('=')
        b.WriteString(quoteIfNeeded(m[k]))
        b.WriteByte('\n')
    }
    return b.String()
}

// Decode parses a .env stream into a map. Returns the line number of the
// first parse error encountered.
func Decode(r io.Reader) (map[string]string, error) {
    out := make(map[string]string)
    scanner := bufio.NewScanner(r)
    line := 0

    for scanner.Scan() {
        line++
        raw := strings.TrimSpace(scanner.Text())

        // Skip empty lines and full-line comments.
        if raw == "" || strings.HasPrefix(raw, "#") {
            continue
        }

        eq := strings.IndexByte(raw, '=')
        if eq < 0 {
            return nil, fmt.Errorf("line %d: missing '='", line)
        }

        key := strings.TrimSpace(raw[:eq])
        val := raw[eq+1:]

        if !validKey(key) {
            return nil, fmt.Errorf("line %d: invalid key %q", line, key)
        }

        decoded, err := decodeValue(val)
        if err != nil {
            return nil, fmt.Errorf("line %d: %w", line, err)
        }

        out[key] = decoded
    }

    if err := scanner.Err(); err != nil {
        return nil, err
    }
    return out, nil
}

// --- internal helpers ---

// validKey checks that key follows shell variable conventions:
// non-empty, starts with letter/underscore, only A-Z, a-z, 0-9, _.
func validKey(s string) bool {
    if s == "" {
        return false
    }
    for i, r := range s {
        if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
            continue
        }
        if i > 0 && r >= '0' && r <= '9' {
            continue
        }
        return false
    }
    return true
}

// decodeValue handles unquoted, double-quoted, and single-quoted values.
func decodeValue(s string) (string, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return "", nil
    }

    switch s[0] {
    case '"':
        return decodeDoubleQuoted(s)
    case '\'':
        return decodeSingleQuoted(s)
    default:
        // Bare value — reject things that look like expansion attempts.
        if strings.ContainsAny(s, "$`") {
            return "", fmt.Errorf("bare value contains $ or ` (quote it explicitly to keep the literal character)")
        }
        return s, nil
    }
}

func decodeDoubleQuoted(s string) (string, error) {
    if len(s) < 2 || s[len(s)-1] != '"' {
        return "", fmt.Errorf("unterminated double-quoted value")
    }
    body := s[1 : len(s)-1]

    var b strings.Builder
    for i := 0; i < len(body); i++ {
        c := body[i]
        if c != '\\' {
            b.WriteByte(c)
            continue
        }
        if i+1 >= len(body) {
            return "", fmt.Errorf("trailing backslash in double-quoted value")
        }
        i++
        switch body[i] {
        case 'n':
            b.WriteByte('\n')
        case 't':
            b.WriteByte('\t')
        case 'r':
            b.WriteByte('\r')
        case '\\':
            b.WriteByte('\\')
        case '"':
            b.WriteByte('"')
        case '$':
            b.WriteByte('$')
        default:
            return "", fmt.Errorf("unknown escape sequence \\%c", body[i])
        }
    }
    return b.String(), nil
}

func decodeSingleQuoted(s string) (string, error) {
    if len(s) < 2 || s[len(s)-1] != '\'' {
        return "", fmt.Errorf("unterminated single-quoted value")
    }
    return s[1 : len(s)-1], nil
}

// quoteIfNeeded returns s as-is if it's a "simple" value (no whitespace,
// no special chars), otherwise double-quoted with escapes.
func quoteIfNeeded(s string) string {
    if s == "" {
        return ""
    }
    needsQuote := false
    for _, r := range s {
        if r == ' ' || r == '\t' || r == '\n' || r == '\r' ||
            r == '"' || r == '\\' || r == '$' || r == '`' || r == '#' {
            needsQuote = true
            break
        }
    }
    if !needsQuote {
        return s
    }

    var b strings.Builder
    b.WriteByte('"')
    for _, r := range s {
        switch r {
        case '\n':
            b.WriteString(`\n`)
        case '\t':
            b.WriteString(`\t`)
        case '\r':
            b.WriteString(`\r`)
        case '\\':
            b.WriteString(`\\`)
        case '"':
            b.WriteString(`\"`)
        default:
            b.WriteRune(r)
        }
    }
    b.WriteByte('"')
    return b.String()
}