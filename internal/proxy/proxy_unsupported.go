//go:build !linux

package proxy

import (
	"errors"
	"time"

	"github.com/dev-boz/codesnips/internal/snippets"
)

type Config struct {
	Store           *snippets.Store
	Command         []string
	RequestedHeight int
	Interval        time.Duration
	HeaderStyle     HeaderStyle
	HeaderReverse   bool
}

type HeaderStyle string

const (
	HeaderStyleText    HeaderStyle = "text"
	HeaderStyleSolid   HeaderStyle = "solid"
	DefaultHeaderStyle             = HeaderStyleSolid
)

func Run(config Config) (int, error) {
	return 2, errors.New("snips wrap is currently supported on Linux only")
}

func (s HeaderStyle) Valid() bool {
	switch s {
	case HeaderStyleText, HeaderStyleSolid:
		return true
	default:
		return false
	}
}
