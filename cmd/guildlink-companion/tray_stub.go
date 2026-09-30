//go:build darwin && !cgo

package main

import (
	"context"

	"guildlink/companion/internal/syncer"
)

// Built without cgo, macOS has no tray icon; the app runs until it is
// stopped and is managed from the settings page.
const trayAvailable = false

func runTray(ctx context.Context, _ context.CancelFunc, _ string, _ *syncer.Syncer) { <-ctx.Done() }
