// Command guildlink-companion uploads the GuildLink addon's saved data to the
// GuildLink Discord bot. It runs in the background with a tray icon and a
// settings page on http://127.0.0.1:47631/.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"guildlink/companion/internal/config"
	"guildlink/companion/internal/syncer"
	"guildlink/companion/internal/web"
	"guildlink/companion/internal/wow"
)

// Set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	headless := flag.Bool("headless", false, "run without a tray icon (stop with Ctrl+C)")
	noBrowser := flag.Bool("no-browser", false, "do not open the settings page on start")
	cfgPath := flag.String("config", "", "settings file (default: the user config folder)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	if *cfgPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			log.Fatalf("finding config folder: %v", err)
		}
		*cfgPath = p
	}
	setupLogging(filepath.Dir(*cfgPath))

	// A second copy would upload everything twice; show the running one instead.
	if alreadyRunning() {
		url := fmt.Sprintf("http://127.0.0.1:%d/", web.PreferredPort)
		log.Printf("already running at %s", url)
		if !*noBrowser {
			openBrowser(url)
		}
		return
	}

	store, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("loading settings from %s: %v", *cfgPath, err)
	}
	autodetectWowDir(store)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s := syncer.New(store, version)
	go s.Run(ctx)

	srv, err := web.Listen(store, s, version)
	if err != nil {
		log.Fatalf("starting settings page: %v", err)
	}
	log.Printf("GuildLink Companion %s; settings at %s", version, srv.URL())

	settings := store.Get()
	if !*noBrowser && (settings.OpenOnLaunch || !settings.Ready()) {
		openBrowser(srv.URL())
	}

	if *headless || !trayAvailable {
		<-ctx.Done()
	} else {
		runTray(ctx, cancel, srv.URL(), s)
	}

	shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	_ = srv.Close(shutdownCtx)
}

func setupLogging(dir string) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "companion.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	// Windows release builds have no console, so the file is the only log there.
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

func alreadyRunning() bool {
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(web.PreferredPort) + "/status")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// autodetectWowDir fills in the WoW folder on first run, preferring an
// install that already has GuildLink saved data.
func autodetectWowDir(store *config.Store) {
	s := store.Get()
	if s.WowDir != "" {
		return
	}
	installs := wow.DetectInstalls()
	if len(installs) == 0 {
		return
	}
	s.WowDir = installs[0]
	for _, dir := range installs {
		if found, _ := wow.FindSavedVariables(dir); len(found) > 0 {
			s.WowDir = dir
			break
		}
	}
	if err := store.Save(s); err != nil {
		log.Printf("saving detected WoW folder: %v", err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open a browser (%v); visit %s", err, url)
		return
	}
	go func() { _ = cmd.Wait() }()
}
