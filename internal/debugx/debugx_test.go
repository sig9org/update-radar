package debugx

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrintf(t *testing.T) {
	var buf bytes.Buffer
	old := Writer
	Writer = &buf
	defer func() { Writer = old }()

	Enable(false)
	Printf("hidden %s", "value")
	if buf.Len() != 0 {
		t.Fatalf("expected no output when disabled, got %q", buf.String())
	}

	Enable(true)
	defer Enable(false)
	Printf("visible %s", "value")
	out := buf.String()
	if !strings.Contains(out, " [DEBUG] ") {
		t.Errorf("output = %q, want a [DEBUG] marker", out)
	}
	if !strings.Contains(out, "visible value") {
		t.Errorf("output = %q, want it to contain the debug line", out)
	}
}

func TestPrintfIncludesTimestamp(t *testing.T) {
	var buf bytes.Buffer
	old := Writer
	Writer = &buf
	defer func() { Writer = old }()

	Enable(true)
	defer Enable(false)
	Printf("hello")

	out := strings.TrimSpace(buf.String())
	fields := strings.SplitN(out, " ", 3)
	if len(fields) != 3 || fields[2] != "[DEBUG] hello" {
		t.Fatalf("output = %q, want a timestamp followed by [DEBUG] and the message", buf.String())
	}
	if _, err := time.Parse("2006-01-02 15:04:05", fields[0]+" "+fields[1]); err != nil {
		t.Errorf("timestamp %q does not match the expected format: %v", fields[0]+" "+fields[1], err)
	}
}

func TestEnabled(t *testing.T) {
	Enable(true)
	if !Enabled() {
		t.Error("Enabled() = false after Enable(true)")
	}
	Enable(false)
	if Enabled() {
		t.Error("Enabled() = true after Enable(false)")
	}
}
