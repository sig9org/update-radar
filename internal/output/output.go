// Package output provides level-colored writers for github-rader's CLI
// messages: no color for normal output, orange for warnings, red for
// errors, gray for debug output. Color is only emitted when w is a
// terminal and the NO_COLOR convention (https://no-color.org/) is not
// requested, so redirected/piped output stays plain text.
package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// TimestampLayout is the timestamp format shared by debug and user-facing
// runtime messages.
const TimestampLayout = "2006-01-02T15:04:05.000Z07:00"

// Timestamp returns the current local time in TimestampLayout.
func Timestamp() string {
	return time.Now().Format(TimestampLayout)
}

// Timestamped prefixes msg with the same timestamp format used by debug
// output.
func Timestamped(msg string) string {
	return Timestamp() + " " + msg
}

// Level selects the ANSI color applied to a message.
type Level int

const (
	Normal Level = iota
	Warning
	Error
	Debug
)

const reset = "\x1b[0m"

func colorCode(level Level) string {
	switch level {
	case Warning:
		return "\x1b[38;5;208m" // orange
	case Error:
		return "\x1b[31m" // red
	case Debug:
		return "\x1b[90m" // gray
	default:
		return ""
	}
}

// isTerminal reports whether w is a character device, so plain files and
// pipes never receive color escapes.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func colorize(w io.Writer, level Level, msg string) string {
	code := colorCode(level)
	if code == "" || os.Getenv("NO_COLOR") != "" || !isTerminal(w) {
		return msg
	}
	return code + msg + reset
}

// Fprintf writes a level-colored, printf-formatted message to w with no
// trailing newline.
func Fprintf(w io.Writer, level Level, format string, args ...any) {
	fmt.Fprint(w, colorize(w, level, fmt.Sprintf(format, args...)))
}

// Fprintln writes a level-colored message to w, followed by a newline.
func Fprintln(w io.Writer, level Level, args ...any) {
	msg := strings.TrimSuffix(fmt.Sprintln(args...), "\n")
	fmt.Fprintln(w, colorize(w, level, msg))
}
