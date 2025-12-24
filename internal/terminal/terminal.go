package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// Terminal wraps a PTY with vt10x terminal emulation
type Terminal struct {
	vt   vt10x.Terminal
	ptmx *os.File
	cmd  *exec.Cmd
	mu   sync.Mutex

	width  int
	height int

	// Channel to signal updates
	Updates chan struct{}
	closed  bool
}

// New creates a new terminal with given dimensions
func New(width, height int) *Terminal {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	return &Terminal{
		width:   width,
		height:  height,
		Updates: make(chan struct{}, 1), // Buffered to avoid blocking
	}
}

// start spawns the shell and begins the terminal session
func (t *Terminal) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create vt10x terminal
	t.vt = vt10x.New(vt10x.WithSize(t.width, t.height))

	// Get shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	t.cmd = exec.Command(shell)
	t.cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
	)

	var err error
	t.ptmx, err = pty.StartWithSize(t.cmd, &pty.Winsize{
		Rows: uint16(t.height),
		Cols: uint16(t.width),
	})
	if err != nil {
		return err
	}

	// keep reading from PTY and feeding vt10x
	go t.readLoop()

	return nil
}

// readLoop reads from PTY and writes to vt10x terminal
func (t *Terminal) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := t.ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				// PTY closed
			}
			return
		}

		t.mu.Lock()
		if t.vt != nil {
			t.vt.Write(buf[:n])
		}
		closed := t.closed
		t.mu.Unlock()

		// update the terminal display
		if !closed {
			select {
			case t.Updates <- struct{}{}:
			default:
				// Channel full, update already pending
			}
		}
	}
}

// Write sends input to the PTY
func (t *Terminal) Write(data []byte) (int, error) {
	t.mu.Lock()
	ptmx := t.ptmx
	t.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}
	return ptmx.Write(data)
}

// Render returns the current terminal content with colors and cursor
// thanks to AI for this
func (t *Terminal) Render() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.vt == nil {
		return ""
	}

	cols, rows := t.vt.Size()
	cursor := t.vt.Cursor()
	cursorVisible := t.vt.CursorVisible()

	var sb strings.Builder
	sb.Grow(cols * rows * 2) // Pre-allocate

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			cell := t.vt.Cell(x, y)
			char := cell.Char
			if char == 0 {
				char = ' '
			}

			// Check if this is cursor position
			isCursor := cursorVisible && x == cursor.X && y == cursor.Y

			// Build ANSI sequence for colors
			fg := cell.FG
			bg := cell.BG

			if isCursor {
				// Swap fg/bg for cursor (reverse video effect)
				fg, bg = bg, fg
			}

			// Write color codes if not default
			hasStyle := false
			if fg != 0 && fg < 256 {
				sb.WriteString(fgColor(fg))
				hasStyle = true
			}
			if bg != 0 && bg < 256 {
				sb.WriteString(bgColor(bg))
				hasStyle = true
			}
			if isCursor && !hasStyle {
				// Fallback reverse video for cursor
				sb.WriteString("\x1b[7m")
				hasStyle = true
			}

			sb.WriteRune(char)

			// Reset if we had styles
			if hasStyle {
				sb.WriteString("\x1b[0m")
			}
		}
		if y < rows-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// fgColor returns ANSI foreground color code
func fgColor(c vt10x.Color) string {
	if c < 8 {
		return fmt.Sprintf("\x1b[%dm", 30+c)
	} else if c < 16 {
		return fmt.Sprintf("\x1b[%dm", 90+(c-8))
	}
	return fmt.Sprintf("\x1b[38;5;%dm", c)
}

// bgColor returns ANSI background color code
func bgColor(c vt10x.Color) string {
	if c < 8 {
		return fmt.Sprintf("\x1b[%dm", 40+c)
	} else if c < 16 {
		return fmt.Sprintf("\x1b[%dm", 100+(c-8))
	}
	return fmt.Sprintf("\x1b[48;5;%dm", c)
}

// Resize changes terminal dimensions
func (t *Terminal) Resize(width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if width < 1 || height < 1 {
		return
	}

	t.width = width
	t.height = height

	if t.vt != nil {
		t.vt.Resize(width, height)
	}

	if t.ptmx != nil {
		pty.Setsize(t.ptmx, &pty.Winsize{
			Rows: uint16(height),
			Cols: uint16(width),
		})
	}
}

// Close shuts down the terminal
func (t *Terminal) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true

	if t.Updates != nil {
		close(t.Updates)
		t.Updates = nil
	}

	if t.ptmx != nil {
		t.ptmx.Close()
		t.ptmx = nil
	}

	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}

	return nil
}

// Size returns current dimensions
func (t *Terminal) Size() (width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.width, t.height
}
