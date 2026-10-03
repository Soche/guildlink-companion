package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// CompanionAssetName is this build's binary in a companion release.
func CompanionAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("guildlink-companion-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// ReplaceExecutable puts a new binary where the running one is. Unix can
// rename over a running file; Windows can't, but it can rename the running
// file aside first (removed by CleanupOldExecutable on the next start).
func ReplaceExecutable(data []byte) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	return replaceFile(exe, data, runtime.GOOS == "windows")
}

func replaceFile(exe string, data []byte, windows bool) error {
	staged := exe + ".new"
	if err := os.WriteFile(staged, data, 0o755); err != nil {
		return fmt.Errorf("can't write next to %s (move the companion to a folder you own): %w", exe, err)
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return err
	}
	if windows {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			_ = os.Remove(staged)
			return err
		}
		if err := os.Rename(staged, exe); err != nil {
			_ = os.Rename(old, exe)
			return err
		}
		return nil
	}
	return os.Rename(staged, exe)
}

// CleanupOldExecutable removes what a previous update left behind.
func CleanupOldExecutable() {
	if exe, err := executablePath(); err == nil {
		_ = os.Remove(exe + ".old")
		_ = os.Remove(exe + ".new")
	}
}

// StartExecutable launches the (new) binary with args; the caller exits after.
func StartExecutable(args []string) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
