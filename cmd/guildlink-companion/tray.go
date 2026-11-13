//go:build !darwin || cgo

package main

import (
	"context"
	"time"

	"fyne.io/systray"

	"guildlink/companion/internal/i18n"
	"guildlink/companion/internal/syncer"
)

// The macOS tray needs cgo; Windows and Linux (StatusNotifier over D-Bus) don't.
const trayAvailable = true

// lang says which language to use; the menu follows it when it changes.
func runTray(ctx context.Context, cancel context.CancelFunc, url string, s *syncer.Syncer, lang func() string) {
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()
	systray.Run(func() {
		systray.SetIcon(trayIcon())
		systray.SetTooltip("GuildLink Companion")
		l := lang()
		status := systray.AddMenuItem(i18n.T(l, "trayStarting"), "")
		status.Disable()
		open := systray.AddMenuItem(i18n.T(l, "trayOpen"), i18n.T(l, "trayOpenTip"))
		syncNow := systray.AddMenuItem(i18n.T(l, "traySync"), i18n.T(l, "traySyncTip"))
		systray.AddSeparator()
		quit := systray.AddMenuItem(i18n.T(l, "trayQuit"), i18n.T(l, "trayQuitTip"))
		shown := l

		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				if l := lang(); l != shown {
					shown = l
					open.SetTitle(i18n.T(l, "trayOpen"))
					open.SetTooltip(i18n.T(l, "trayOpenTip"))
					syncNow.SetTitle(i18n.T(l, "traySync"))
					syncNow.SetTooltip(i18n.T(l, "traySyncTip"))
					quit.SetTitle(i18n.T(l, "trayQuit"))
					quit.SetTooltip(i18n.T(l, "trayQuitTip"))
				}
				status.SetTitle(summary(s.Status(), shown))
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

func summary(st syncer.Status, lang string) string {
	if st.Problem != "" {
		return i18n.T(lang, "trayNeedsSetup")
	}
	var last time.Time
	for _, f := range st.Files {
		if f.LastError != "" {
			return i18n.T(lang, "trayFailing")
		}
		if f.LastSync.After(last) {
			last = f.LastSync
		}
	}
	if last.IsZero() {
		return i18n.T(lang, "trayWaiting")
	}
	return i18n.T(lang, "trayLast", "time", last.Format("15:04"))
}
