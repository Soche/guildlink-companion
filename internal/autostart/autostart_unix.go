//go:build !windows && !darwin

package autostart

import (
	"os"
	"path/filepath"
)

func desktopPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "guildlink-companion.desktop"), nil
}

func enabled() (bool, error) {
	p, err := desktopPath()
	return err == nil && fileExists(p), err
}

func enable(exe string) error {
	p, err := desktopPath()
	if err != nil {
		return err
	}
	return writeFile(p, []byte(desktopEntry(exe)))
}

func disable() error {
	p, err := desktopPath()
	if err != nil {
		return err
	}
	return removeFile(p)
}
