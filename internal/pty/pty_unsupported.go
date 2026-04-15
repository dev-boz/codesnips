//go:build !linux

package pty

import (
	"errors"
	"os"
	"syscall"
)

var errUnsupported = errors.New("snips wrap is currently supported on Linux only")

type Child struct {
	Master *os.File
	PID    int
}

type TerminalMode struct{}

func Supported() bool {
	return false
}

func Start(argv []string, rows, cols int) (*Child, error) {
	return nil, errUnsupported
}

func IsTTY(fd uintptr) bool {
	return true
}

func TerminalSize(fd int) (int, int, error) {
	return 0, 0, errUnsupported
}

func SetSize(file *os.File, rows, cols int) error {
	return errUnsupported
}

func EnableRawMode(fd int) (*TerminalMode, error) {
	return nil, errUnsupported
}

func (m *TerminalMode) Restore() {}

func SignalGroup(pid int, sig syscall.Signal) {}
