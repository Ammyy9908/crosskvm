package config

import (
	"errors"
	"fmt"
	"strings"
)

// ScreenEdge represents the boundary of the screen configured to trigger control handoff.
type ScreenEdge string

const (
	EdgeNone   ScreenEdge = "none"
	EdgeLeft   ScreenEdge = "left"
	EdgeRight  ScreenEdge = "right"
	EdgeTop    ScreenEdge = "top"
	EdgeBottom ScreenEdge = "bottom"
)

// Mode represents the running mode of the node.
type Mode string

const (
	ModeListen  Mode = "listen"
	ModeConnect Mode = "connect"
)

var (
	ErrInvalidMode = errors.New("config: mode must be either 'listen' or 'connect'")
	ErrMissingAddr = errors.New("config: address cannot be empty")
)

// Config holds runtime configuration settings for CrossKVM.
type Config struct {
	Mode         Mode       `json:"mode"`
	Addr         string     `json:"addr"`
	ScreenEdge   ScreenEdge `json:"screen_edge"`
	EmergencyKey string     `json:"emergency_key"`
	Debug        bool       `json:"debug"`
}

// DefaultConfig returns standard default configurations.
func DefaultConfig() Config {
	return Config{
		Mode:         ModeListen,
		Addr:         ":4545",
		ScreenEdge:   EdgeRight,
		EmergencyKey: "Escape",
		Debug:        false,
	}
}

// Validate checks the configuration for errors.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeListen, ModeConnect:
	default:
		return fmt.Errorf("%w: got '%s'", ErrInvalidMode, c.Mode)
	}

	if strings.TrimSpace(c.Addr) == "" {
		return ErrMissingAddr
	}

	return nil
}
