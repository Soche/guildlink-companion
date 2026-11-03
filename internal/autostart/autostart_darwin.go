package autostart

import (
	"os"
	"path/filepath"
)

const label = "top.lassi.guildlink.companion"

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func enabled() (bool, error) {
	p, err := plistPath()
	return err == nil && fileExists(p), err
}

func enable(exe string) error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	return writeFile(p, []byte(launchAgent(label, exe)))
}

func disable() error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	return removeFile(p)
}
