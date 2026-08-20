// Package debugx provides a process-wide toggle for verbose debug logging.
package debugx

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/sig9org/update-radar/internal/output"
)

var enabled atomic.Bool

// Writer is where debug output is written. Overridable in tests.
var Writer io.Writer = os.Stdout

// Enable turns debug logging on or off.
func Enable(v bool) {
	enabled.Store(v)
}

// Enabled reports whether debug logging is currently on.
func Enabled() bool {
	return enabled.Load()
}

// Printf writes a debug line to Writer when debug logging is enabled. The
// line is colored gray when Writer is a terminal; the underlying "[debug]
// <timestamp> <message>" text is unchanged so it stays in sync with
// chatxgo's own internal/debugx format.
func Printf(format string, args ...any) {
	if !enabled.Load() {
		return
	}
	msg := time.Now().Format("2006-01-02 15:04:05") + " [DEBUG] " + fmt.Sprintf(format, args...)
	output.Fprintln(Writer, output.Debug, msg)
}
