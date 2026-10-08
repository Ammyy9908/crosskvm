package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	localDeviceMu sync.RWMutex
	cachedID      string
	cachedDevice  *LocalDevice
)

// LocalDevice represents this machine's stable network identity.
type LocalDevice struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	Port         int      `json:"port"`
	ScreenWidth  int      `json:"screen_width"`
	ScreenHeight int      `json:"screen_height"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

func tryConfigDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	probe := filepath.Join(dir, ".write_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0600); err != nil {
		return "", err
	}
	_ = os.Remove(probe)
	return dir, nil
}

// GetConfigDir returns the OS-specific directory for CrossKVM state and config.
func GetConfigDir() (string, error) {
	if custom := os.Getenv("CROSSKVM_DIR"); custom != "" {
		if d, err := tryConfigDir(custom); err == nil {
			return d, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
	}

	if home != "" {
		var dir string
		if runtime.GOOS == "windows" {
			appData := os.Getenv("APPDATA")
			if appData != "" {
				dir = filepath.Join(appData, "crosskvm")
			} else {
				dir = filepath.Join(home, ".crosskvm")
			}
		} else {
			dir = filepath.Join(home, ".crosskvm")
		}
		if d, err := tryConfigDir(dir); err == nil {
			return d, nil
		}
	}

	// Fallback to local .crosskvm directory in current workspace
	localDir := ".crosskvm"
	if d, err := tryConfigDir(localDir); err == nil {
		return d, nil
	}

	return "", errors.New("cannot create writable crosskvm state directory")
}

// GetOrCreateDeviceID returns a stable device ID, creating and saving one if none exists.
func GetOrCreateDeviceID() (string, error) {
	localDeviceMu.Lock()
	defer localDeviceMu.Unlock()

	if cachedID != "" {
		return cachedID, nil
	}

	configDir, err := GetConfigDir()
	if err == nil {
		idPath := filepath.Join(configDir, "device_id")
		if data, err := os.ReadFile(idPath); err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				cachedID = id
				return id, nil
			}
		}
		newID := generateRandomDeviceID()
		_ = os.WriteFile(idPath, []byte(newID), 0600)
		cachedID = newID
		return newID, nil
	}

	cachedID = generateRandomDeviceID()
	return cachedID, nil
}

func generateRandomDeviceID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("xkvm-%d", os.Getpid())
	}
	return fmt.Sprintf("xkvm-%s", hex.EncodeToString(b))
}

// GetDeviceName returns the system hostname or a clean fallback.
func GetDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = fmt.Sprintf("crosskvm-%s", runtime.GOOS)
	}
	return hostname
}

// GetLocalDevice returns or creates the local device profile.
func GetLocalDevice(port, screenWidth, screenHeight int, version string) (*LocalDevice, error) {
	id, err := GetOrCreateDeviceID()
	if err != nil {
		id = generateRandomDeviceID()
	}

	name := GetDeviceName()
	localDeviceMu.Lock()
	defer localDeviceMu.Unlock()

	cachedDevice = &LocalDevice{
		ID:           id,
		Name:         name,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Port:         port,
		ScreenWidth:  screenWidth,
		ScreenHeight: screenHeight,
		Version:      version,
		Capabilities: []string{"input", "control", "heartbeat"},
	}

	return cachedDevice, nil
}
