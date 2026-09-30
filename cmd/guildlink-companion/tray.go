//go:build !darwin || cgo

package main

import (
	"context"
	"fmt"
	"time"

	"fyne.io/systray"

	"guildlink/companion/internal/syncer"
)

// The macOS tray needs cgo; Windows and Linux (StatusNotifier over D-Bus) don't.
const trayAvailable = true

func runTray(ctx context.Context, cancel context.CancelFunc, url string, s *syncer.Syncer) {
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()
	systray.Run(func() {
		systray.SetIcon(trayIcon())
		systray.SetTooltip("GuildLink Companion")
		status := systray.AddMenuItem("Starting…", "")
		status.Disable()
		open := systray.AddMenuItem("Open GuildLink", "Settings and upload status")
		syncNow := systray.AddMenuItem("Sync now", "Upload saved data again")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop uploading")

		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				status.SetTitle(summary(s.Status()))
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				case <-open.ClickedCh:
					openBrowser(url)
				case <-syncNow.ClickedCh:
					s.SyncNow()
				case <-quit.ClickedCh:
					cancel()
					return
				}
			}
		}()
	}, func() {})
}

func summary(st syncer.Status) string {
	if st.Problem != "" {
		return "Needs setup: open GuildLink"
	}
	var last time.Time
	for _, f := range st.Files {
		if f.LastError != "" {
			return "Upload failing: open GuildLink"
		}
		if f.LastSync.After(last) {
			last = f.LastSync
		}
	}
	if last.IsZero() {
		return "Waiting for saved data"
	}
	return fmt.Sprintf("Last upload %s", last.Format("15:04"))
}
