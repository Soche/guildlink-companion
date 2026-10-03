package update

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	addonFolder = "GuildLink"
	seedFile    = "Seed.lua"
)

// AddonAssetName is the zip the addon's release workflow attaches.
func AddonAssetName(tag string) string { return "GuildLink-" + tag + ".zip" }

// InstalledAddonVersion reads "## Version:" from the installed addon's TOC,
// e.g. "v0.5.0". ok is false when the addon isn't installed in addonsDir.
func InstalledAddonVersion(addonsDir string) (version string, ok bool) {
	f, err := os.Open(filepath.Join(addonsDir, addonFolder, addonFolder+".toc"))
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, found := strings.CutPrefix(sc.Text(), "## Version:"); found {
			return "v" + strings.TrimPrefix(strings.TrimSpace(v), "v"), true
		}
	}
	return "", true
}

// InstallAddon replaces addonsDir/GuildLink with the zip's GuildLink folder.
// The zip is fully extracted beside the old folder before anything is
// swapped, the previous Seed.lua (saved data, see the addon README) is kept,
// and on failure the old folder is put back.
func InstallAddon(zipData []byte, addonsDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("reading addon zip: %w", err)
	}
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(addonsDir, addonFolder)
	staging := target + ".new"
	backup := target + ".old"
	_ = os.RemoveAll(staging)
	_ = os.RemoveAll(backup)

	if err := extractAddon(zr, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	if seed, err := os.ReadFile(filepath.Join(target, seedFile)); err == nil {
		if err := os.WriteFile(filepath.Join(staging, seedFile), seed, 0o644); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
	}

	hadOld := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("moving the old addon aside (is a file in it open?): %w", err)
		}
		hadOld = true
	}
	if err := os.Rename(staging, target); err != nil {
		if hadOld {
			_ = os.Rename(backup, target)
		}
		_ = os.RemoveAll(staging)
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

// extractAddon writes the zip's GuildLink/ entries into dest. Anything
// outside that folder, or any path trying to escape it, is rejected.
func extractAddon(zr *zip.Reader, dest string) error {
	found := false
	for _, f := range zr.File {
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		rel, ok := strings.CutPrefix(name, addonFolder+"/")
		if name == addonFolder {
			continue
		}
		if !ok || rel == "" || strings.HasPrefix(rel, "../") || path.IsAbs(rel) || strings.Contains(rel, "/../") {
			return fmt.Errorf("addon zip has an unexpected entry %q", f.Name)
		}
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("addon zip has a non-regular file %q", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := extractFile(f, out); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return errors.New("addon zip has no GuildLink folder")
	}
	if _, err := os.Stat(filepath.Join(dest, addonFolder+".toc")); err != nil {
		return errors.New("addon zip has no GuildLink.toc")
	}
	return nil
}

func extractFile(f *zip.File, out string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, io.LimitReader(rc, maxDownload)); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
