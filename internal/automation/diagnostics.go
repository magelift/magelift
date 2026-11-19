package automation

import (
	"bytes"
	"strings"
	"sync"

	"github.com/magelift/magelift/internal/secretsafe"
)

// Diagnostics holds a bounded, redacted tail of Pulumi progress for the operator.
// An oversized line is dropped in full so truncation cannot split a credential.
type Diagnostics struct {
	mu        sync.Mutex
	lines     []string
	pending   []byte
	oversized bool
}

func (d *Diagnostics) Write(data []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	length := len(data)
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		end := i
		if i < 0 {
			end = len(data)
		}
		if len(d.pending)+end > 4096 {
			d.oversized = true
			d.pending = nil
		}
		if !d.oversized {
			d.pending = append(d.pending, data[:end]...)
		}
		if i < 0 {
			break
		}
		if line := d.line(); line != "" {
			d.lines = append(d.lines, line)
			if len(d.lines) > 50 {
				d.lines = d.lines[1:]
			}
		}
		d.pending = nil
		d.oversized = false
		data = data[i+1:]
	}
	return length, nil
}

func (d *Diagnostics) line() string {
	if d.oversized {
		return "[long diagnostic line omitted]"
	}
	line, _ := secretsafe.RedactSensitiveText(strings.TrimSpace(string(d.pending)))
	return line
}

func (d *Diagnostics) Lines() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snapshot()
}

func (d *Diagnostics) snapshot() []string {
	lines := append([]string(nil), d.lines...)
	if line := d.line(); line != "" {
		lines = append(lines, line)
		if len(lines) > 50 {
			lines = lines[1:]
		}
	}
	return lines
}

// Drain returns diagnostics for a completed deployment phase.
func (d *Diagnostics) Drain() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	lines := d.snapshot()
	d.pending = nil
	d.lines = nil
	d.oversized = false
	return lines
}
