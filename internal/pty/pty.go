//go:build linux

package pty

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	creackpty "github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type Child struct {
	Master *os.File
	PID    int
}

type TerminalMode struct {
	fd       int
	original *terminalState
	restored bool
	mu       sync.Mutex
}

type terminalState struct {
	termios unix.Termios
}

func Supported() bool {
	return true
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
	_, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	return err == nil
}

func TerminalSize(fd int) (int, int, error) {
	size, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}
	cols := int(size.Col)
	rows := int(size.Row)
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
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}

	original := &terminalState{termios: *termios}
	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB
	termios.Cflag |= unix.CS8
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
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
	_ = unix.IoctlSetTermios(m.fd, unix.TCSETS, &m.original.termios)
	m.restored = true
}

func SignalGroup(pid int, sig syscall.Signal) {
	_ = syscall.Kill(-pid, sig)
}
