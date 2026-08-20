package logx

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestSeverityColors(t *testing.T) {
	var out, stderr bytes.Buffer
	logger := Logger{Out: &out, Err: &stderr}
	logger.Infof("normal")
	logger.Warnf("warning")
	logger.Errorf("error")
	timestamped := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} (normal|warning|error)$`)
	if !timestamped.MatchString(strings.TrimSpace(out.String())) {
		t.Fatalf("normal output = %q", out.String())
	}
	plainStderr := strings.NewReplacer(orange, "", red, "", reset, "").Replace(stderr.String())
	lines := strings.Split(strings.TrimSpace(plainStderr), "\n")
	if len(lines) != 2 || !timestamped.MatchString(lines[0]) || !timestamped.MatchString(lines[1]) ||
		!strings.Contains(stderr.String(), orange) || !strings.Contains(stderr.String(), red) {
		t.Fatalf("severity output = %q", stderr.String())
	}
}

func TestDebugOverridesSilent(t *testing.T) {
	var out bytes.Buffer
	logger := Logger{Out: &out, Silent: true, Debug: true}
	logger.Infof("normal")
	logger.Debugf("details")
	if !strings.Contains(out.String(), "normal") || !strings.Contains(out.String(), "[DEBUG] details") ||
		!strings.Contains(out.String(), gray) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSuccessUsesGreenOutput(t *testing.T) {
	var out bytes.Buffer
	logger := Logger{Out: &out}
	logger.Successf("success")
	if !strings.Contains(out.String(), green+"2026-") || !strings.Contains(out.String(), " success"+reset) {
		t.Fatalf("success output = %q", out.String())
	}
}
