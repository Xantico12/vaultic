package dotenv

import (
	"strings"
	"testing"
)

func TestEncodeBasic(t *testing.T) {
    in := map[string]string{
        "FOO":   "bar",
        "EMPTY": "",
        "BAZ":   "qux",
    }
    got := Encode(in)
    want := "BAZ=qux\nEMPTY=\nFOO=bar\n"
    if got != want {
        t.Errorf("Encode() = %q, want %q", got, want)
    }
}

func TestEncodeQuoting(t *testing.T) {
    cases := map[string]string{
        "PLAIN":      "simple",
        "WITH_SPACE": "hello world",
        "WITH_DOLLAR": "price=$5",
        "WITH_NEWLINE": "line1\nline2",
        "WITH_QUOTE":  `say "hi"`,
    }
    out := Encode(cases)

    if !strings.Contains(out, "PLAIN=simple\n") {
        t.Errorf("plain value should not be quoted: %q", out)
    }
    if !strings.Contains(out, `WITH_SPACE="hello world"`) {
        t.Errorf("spaces should trigger quoting: %q", out)
    }
    if !strings.Contains(out, `WITH_NEWLINE="line1\nline2"`) {
        t.Errorf("newline should be escaped: %q", out)
    }
    if !strings.Contains(out, `WITH_QUOTE="say \"hi\""`) {
        t.Errorf("inner quotes should be escaped: %q", out)
    }
}

func TestDecodeBasic(t *testing.T) {
    input := `
# This is a comment
FOO=bar
EMPTY=
QUOTED="hello world"
SINGLE='no $expansion'
ESCAPES="line1\nline2\ttab"
`
    m, err := Decode(strings.NewReader(input))
    if err != nil {
        t.Fatalf("Decode failed: %v", err)
    }

    want := map[string]string{
        "FOO":     "bar",
        "EMPTY":   "",
        "QUOTED":  "hello world",
        "SINGLE":  "no $expansion",
        "ESCAPES": "line1\nline2\ttab",
    }
    for k, v := range want {
        if m[k] != v {
            t.Errorf("%s = %q, want %q", k, m[k], v)
        }
    }
    if len(m) != len(want) {
        t.Errorf("got %d entries, want %d: %v", len(m), len(want), m)
    }
}

func TestRoundtrip(t *testing.T) {
    original := map[string]string{
        "SIMPLE":     "value",
        "WITH_SPACE": "two words",
        "WITH_NL":    "first\nsecond",
        "WITH_TAB":   "a\tb",
        "EMPTY":      "",
        "DOLLAR":     "$LITERAL",
    }
    encoded := Encode(original)
    decoded, err := Decode(strings.NewReader(encoded))
    if err != nil {
        t.Fatalf("decode roundtrip failed: %v\ninput: %s", err, encoded)
    }
    for k, v := range original {
        if decoded[k] != v {
            t.Errorf("roundtrip %s: got %q, want %q", k, decoded[k], v)
        }
    }
}

func TestRejectsExpansion(t *testing.T) {
    cases := []string{
        "VAR=$HOME",
        "CMD=$(date)",
        "BACKTICK=`whoami`",
    }
    for _, c := range cases {
        _, err := Decode(strings.NewReader(c))
        if err == nil {
            t.Errorf("expected error for %q, got nil", c)
        }
    }
}

func TestRejectsBadKeys(t *testing.T) {
    cases := []string{
        "1KEY=value",
        "MY-KEY=value",
        "MY.KEY=value",
        "=value",
    }
    for _, c := range cases {
        _, err := Decode(strings.NewReader(c))
        if err == nil {
            t.Errorf("expected error for %q, got nil", c)
        }
    }
}

func TestRejectsMalformed(t *testing.T) {
    cases := []string{
        "NOEQUALS",
        `UNTERMINATED="hello`,
        `BADESCAPE="hello\q"`,
    }
    for _, c := range cases {
        _, err := Decode(strings.NewReader(c))
        if err == nil {
            t.Errorf("expected error for %q, got nil", c)
        }
    }
}