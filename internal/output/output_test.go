package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// A bytes.Buffer is never a *os.File, so isTerminal is always false in
// these tests and no color escapes should ever be written.

func TestFprintlnNoColorForNonTerminal(t *testing.T) {
	for _, level := range []Level{Normal, Warning, Error, Debug} {
		var buf bytes.Buffer
		Fprintln(&buf, level, "hello", "world")
		if got, want := buf.String(), "hello world\n"; got != want {
			t.Errorf("level %v: Fprintln output = %q, want %q", level, got, want)
		}
	}
}

func TestFprintfNoColorForNonTerminal(t *testing.T) {
	var buf bytes.Buffer
	Fprintf(&buf, Error, "%d problems", 3)
	if got, want := buf.String(), "3 problems"; got != want {
		t.Errorf("Fprintf output = %q, want %q", got, want)
	}
}

func TestFprintlnDoesNotColorNormalLevel(t *testing.T) {
	var buf bytes.Buffer
	Fprintln(&buf, Normal, "plain message")
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("Normal level output contains an ANSI escape: %q", buf.String())
	}
}

func TestTimestamped(t *testing.T) {
	got := Timestamped("hello world")
	fields := strings.SplitN(got, " ", 2)
	if len(fields) != 2 || fields[1] != "hello world" {
		t.Fatalf("Timestamped() = %q, want a timestamp followed by the message", got)
	}
	if _, err := time.Parse(TimestampLayout, fields[0]); err != nil {
		t.Errorf("timestamp %q does not match TimestampLayout: %v", fields[0], err)
	}
}
