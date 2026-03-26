package progress

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

const barWidth = 30

// Bar renders an in-place ASCII progress bar to stderr.
// Update and Done are safe to call concurrently.
//
// Example output:
//
//	services          [=============>        ] 65% (2600/4000)
type Bar struct {
	mu    sync.Mutex
	label string
	total int
	cur   int
}

// New creates a Bar and immediately renders the initial 0% state.
func New(label string, total int) *Bar {
	b := &Bar{label: label, total: total}
	b.render()
	return b
}

// Inc increments the current count by 1 and redraws the bar.
func (b *Bar) Inc() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cur++
	b.render()
}

// Done finalises the bar: fills it to 100% and moves to a new line.
func (b *Bar) Done() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cur = b.total
	b.render()
	fmt.Fprint(os.Stderr, "\n")
}

func (b *Bar) render() {
	pct := 0
	filled := 0
	if b.total > 0 {
		pct = b.cur * 100 / b.total
		filled = b.cur * barWidth / b.total
	}
	arrow := ""
	if filled < barWidth {
		arrow = ">"
	}
	bar := strings.Repeat("=", filled) + arrow + strings.Repeat(" ", barWidth-filled-len(arrow))
	fmt.Fprintf(os.Stderr, "\r  %-24s [%s] %3d%% (%d/%d)",
		b.label, bar, pct, b.cur, b.total)
}
