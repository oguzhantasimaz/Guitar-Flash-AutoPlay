package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings are the choices made in the window, kept between runs.
type Settings struct {
	Keys     [5]string `json:"keys"`
	OffsetMs int       `json:"offset_ms"`
	Hold     bool      `json:"hold"`
	Practice bool      `json:"practice"`
	// Frets are the green and orange fret centres (x1, y1, x2, y2) when they
	// were pointed at by hand; nil means find them automatically.
	Frets *[4]int `json:"frets,omitempty"`
}

func defaultSettings() Settings {
	return Settings{Keys: [5]string{"a", "s", "j", "k", "l"}, Hold: true}
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "GuitarFlashAutoPlay", "settings.json"), nil
}

// loadSettings returns the saved settings, or the defaults.
func loadSettings() Settings {
	s := defaultSettings()
	path, err := settingsPath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if json.Unmarshal(data, &s) != nil {
		return defaultSettings()
	}
	return s
}

func (s Settings) save() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
