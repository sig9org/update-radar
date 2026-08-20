// Package logx provides severity-aware terminal output.
package logx

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	orange = "\x1b[38;5;208m"
	gray   = "\x1b[90m"
	reset  = "\x1b[0m"
)

// Successf writes a green success message unless silent mode is enabled.
func (l *Logger) Successf(format string, args ...any) {
	if l.Silent && !l.Debug {
		return
	}
	l.write(l.Out, green, timestamp()+" "+format, args...)
}

// Logger writes cisco-rader messages. Debug output overrides silent mode.
type Logger struct {
	Out    io.Writer
	Err    io.Writer
	Silent bool
	Debug  bool
	mu     sync.Mutex
}

// Infof writes an uncolored normal message unless silent mode is enabled.
func (l *Logger) Infof(format string, args ...any) {
	if l.Silent && !l.Debug {
		return
	}
	l.write(l.Out, "", timestamp()+" "+format, args...)
}

// Warnf writes an orange warning message.
func (l *Logger) Warnf(format string, args ...any) {
	l.write(l.Err, orange, timestamp()+" "+format, args...)
}

// Errorf writes a red error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.write(l.Err, red, timestamp()+" "+format, args...)
}

// Debugf writes a timestamped gray message when debug mode is enabled.
func (l *Logger) Debugf(format string, args ...any) {
	if !l.Debug {
		return
	}
	prefix := timestamp() + " [DEBUG] "
	l.write(l.Out, gray, prefix+format, args...)
}

func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (l *Logger) write(w io.Writer, color, format string, args ...any) {
	if w == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	message := fmt.Sprintf(format, args...)
	if color != "" {
		message = color + message + reset
	}
	fmt.Fprintln(w, message)
}
