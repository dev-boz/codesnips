package pty

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	creackpty "github.com/creack/pty"
	"golang.org/x/term"
)

type Child struct {
	Master *os.File
	PID    int
}

type TerminalMode struct {
	fd       int
	original *term.State
	restored bool
	mu       sync.Mutex
}

func Start(argv []string, rows, cols int) (*Child, error) {
	if len(argv) == 0 {
		return nil, errors.New("command is required")
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	attrs := &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
	master, err := creackpty.StartWithAttrs(cmd, &creackpty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}, attrs)
	if err != nil {
		return nil, err
	}

	return &Child{
		Master: master,
		PID:    cmd.Process.Pid,
	}, nil
}

func IsTTY(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

func TerminalSize(fd int) (int, int, error) {
	cols, rows, err := term.GetSize(fd)
	if err != nil {
		return 0, 0, err
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	return rows, cols, nil
}

func SetSize(file *os.File, rows, cols int) error {
	return creackpty.Setsize(file, &creackpty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func EnableRawMode(fd int) (*TerminalMode, error) {
	original, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return &TerminalMode{fd: fd, original: original}, nil
}

func (m *TerminalMode) Restore() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restored {
		return
	}
	_ = term.Restore(m.fd, m.original)
	m.restored = true
}

func SignalGroup(pid int, sig syscall.Signal) {
	_ = syscall.Kill(-pid, sig)
}
