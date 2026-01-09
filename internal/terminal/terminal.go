package terminal

import (
	"fmt"
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

	width   int
	height  int
	workDir string // isolated working directory for this terminal

	// Subscriber channels for multi-client broadcast
	subscribers map[chan struct{}]struct{}
	subMu       sync.RWMutex
	closed      bool

	// Render optimization
	lastRender string // cached render output
	dirty      bool   // needs re-render
}

func New(width, height int, workDir string) *Terminal {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}
	if workDir == "" {
		workDir = "/app"
	}

	return &Terminal{
		width:       width,
		height:      height,
		workDir:     workDir,
		subscribers: make(map[chan struct{}]struct{}),
	}
}

// Subscribe creates a new channel for receiving update notifications.
// we call Unsubscribe when done to avoid leaks.
func (t *Terminal) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	t.subMu.Lock()
	t.subscribers[ch] = struct{}{}
	t.subMu.Unlock()
	return ch
}

// Unsubscribe removes a channel from the subscriber list and closes it.
func (t *Terminal) Unsubscribe(ch chan struct{}) {
	t.subMu.Lock()
	delete(t.subscribers, ch)
	t.subMu.Unlock()
}

// broadcast sends an update signal to all subscribers
func (t *Terminal) broadcast() {
	t.subMu.RLock()
	defer t.subMu.RUnlock()

	for ch := range t.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (t *Terminal) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.vt = vt10x.New(vt10x.WithSize(t.width, t.height))

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	t.cmd = exec.Command(shell)
	t.cmd.Dir = t.workDir
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
			// Shell process exited
			t.mu.Lock()
			t.closed = true
			t.mu.Unlock()
			return
		}

		t.mu.Lock()
		if t.vt != nil {
			t.vt.Write(buf[:n])
			t.dirty = true
		}
		closed := t.closed
		t.mu.Unlock()

		// Broadcast to all subscribers
		if !closed {
			t.broadcast()
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

func (t *Terminal) Render() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.vt == nil {
		return ""
	}

	// Return cached render if not dirty
	if !t.dirty && t.lastRender != "" {
		return t.lastRender
	}

	cols, rows := t.vt.Size()
	cursor := t.vt.Cursor()
	cursorVisible := t.vt.CursorVisible()

	var sb strings.Builder
	sb.Grow(cols * rows * 2)

	// Track previous colors for run-length encoding
	var prevFG, prevBG vt10x.Color
	var inStyle bool

	for y := 0; y < rows; y++ {
		prevFG, prevBG = 0, 0
		inStyle = false

		for x := range cols {
			cell := t.vt.Cell(x, y)
			char := cell.Char
			if char == 0 {
				char = ' '
			}

			isCursor := cursorVisible && x == cursor.X && y == cursor.Y

			fg := cell.FG
			bg := cell.BG

			if isCursor {
				// Swap fg/bg for cursor (reverse video effect)
				fg, bg = bg, fg
			}

			needsColorChange := fg != prevFG || bg != prevBG || (isCursor && !inStyle)

			if needsColorChange {
				if inStyle {
					sb.WriteString("\x1b[0m")
					inStyle = false
				}

				if fg != 0 && fg < 256 {
					sb.WriteString(fgColor(fg))
					inStyle = true
				}
				if bg != 0 && bg < 256 {
					sb.WriteString(bgColor(bg))
					inStyle = true
				}
				if isCursor && !inStyle {
					// Fallback reverse video for cursor
					sb.WriteString("\x1b[7m")
					inStyle = true
				}

				prevFG, prevBG = fg, bg
			}

			sb.WriteRune(char)
		}

		if inStyle {
			sb.WriteString("\x1b[0m")
			inStyle = false
		}

		if y < rows-1 {
			sb.WriteString("\n")
		}
	}

	// Cache the result
	t.lastRender = sb.String()
	t.dirty = false

	return t.lastRender
}

func fgColor(c vt10x.Color) string {
	if c < 8 {
		return fmt.Sprintf("\x1b[%dm", 30+c)
	} else if c < 16 {
		return fmt.Sprintf("\x1b[%dm", 90+(c-8))
	}
	return fmt.Sprintf("\x1b[38;5;%dm", c)
}

func bgColor(c vt10x.Color) string {
	if c < 8 {
		return fmt.Sprintf("\x1b[%dm", 40+c)
	} else if c < 16 {
		return fmt.Sprintf("\x1b[%dm", 100+(c-8))
	}
	return fmt.Sprintf("\x1b[48;5;%dm", c)
}

func (t *Terminal) Resize(width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if width < 1 || height < 1 {
		return
	}

	t.width = width
	t.height = height
	t.dirty = true
	t.lastRender = ""

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

func (t *Terminal) Close() error {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()

	// Close all subscriber channels
	t.subMu.Lock()
	for ch := range t.subscribers {
		close(ch)
	}
	t.subscribers = nil
	t.subMu.Unlock()

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ptmx != nil {
		t.ptmx.Close()
		t.ptmx = nil
	}

	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}

	return nil
}

func (t *Terminal) Size() (width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.width, t.height
}
