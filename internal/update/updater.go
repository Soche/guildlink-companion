package update

import (
	"context"
	"log"
	"sync"
	"time"

	"guildlink/companion/internal/config"
	"guildlink/companion/internal/wow"
)

const (
	checkInterval = 6 * time.Hour
	firstCheck    = 5 * time.Second
)

// AddonTarget is the addon's state in one game flavor folder.
type AddonTarget struct {
	Flavor    string `json:"flavor"`
	Installed string `json:"installed"` // "" when not installed
	Error     string `json:"error"`
}

type Status struct {
	CompanionVersion string        `json:"companionVersion"`
	CompanionLatest  string        `json:"companionLatest"`
	CompanionError   string        `json:"companionError"`
	AddonLatest      string        `json:"addonLatest"`
	AddonError       string        `json:"addonError"`
	AddonTargets     []AddonTarget `json:"addonTargets"`
	Flavors          []string      `json:"flavors"`
	AutoUpdate       bool          `json:"autoUpdate"`
	Checking         bool          `json:"checking"`
	LastCheck        time.Time     `json:"lastCheck"`
	// Set after restarting into a new version ("v0.5.0").
	UpdatedFrom string `json:"updatedFrom"`
}

type Updater struct {
	store   *config.Store
	version string
	gh      *GitHub
	// restart is called after the binary was replaced; it should shut down
	// and start the new one.
	restart func()

	trigger chan bool
	mu      sync.Mutex
	status  Status
}

func New(store *config.Store, version, updatedFrom string, restart func()) *Updater {
	return &Updater{
		store:   store,
		version: version,
		gh:      NewGitHub(version),
		restart: restart,
		trigger: make(chan bool, 1),
		status:  Status{CompanionVersion: version, UpdatedFrom: updatedFrom},
	}
}

// Run checks shortly after start and then every few hours until ctx ends.
func (u *Updater) Run(ctx context.Context) {
	timer := time.NewTimer(firstCheck)
	defer timer.Stop()
	for {
		apply := false
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case apply = <-u.trigger:
		}
		u.check(ctx, apply || !u.store.Get().DisableAutoUpdate)
		timer.Reset(checkInterval)
	}
}

// CheckNow checks for updates; with apply it also installs them even when
// automatic updates are off.
func (u *Updater) CheckNow(apply bool) {
	select {
	case u.trigger <- apply:
	default:
	}
}

func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.status
	s.AddonTargets = append([]AddonTarget(nil), s.AddonTargets...)
	s.AutoUpdate = !u.store.Get().DisableAutoUpdate
	if dir := u.store.Get().WowDir; dir != "" {
		s.Flavors = wow.Flavors(dir)
	}
	return s
}

func (u *Updater) set(fn func(s *Status)) {
	u.mu.Lock()
	fn(&u.status)
	u.mu.Unlock()
}

// TargetFlavors is where the addon is kept installed: the user's choice, or
// a sensible default.
func TargetFlavors(s config.Settings) []string {
	if s.WowDir == "" {
		return nil
	}
	if len(s.AddonFlavors) > 0 {
		return s.AddonFlavors
	}
	return wow.DefaultAddonFlavors(s.WowDir)
}

func (u *Updater) check(ctx context.Context, apply bool) {
	u.set(func(s *Status) { s.Checking = true })
	defer u.set(func(s *Status) { s.Checking = false; s.LastCheck = time.Now() })

	u.checkAddon(ctx, apply)
	u.checkCompanion(ctx, apply)
}

func (u *Updater) checkAddon(ctx context.Context, apply bool) {
	settings := u.store.Get()
	release, err := u.gh.Latest(ctx, AddonRepo)
	if err != nil {
		log.Printf("addon update check: %v", err)
		u.set(func(s *Status) { s.AddonError = "Couldn't check for addon updates: " + err.Error() })
		return
	}
	u.set(func(s *Status) { s.AddonLatest = release.Tag; s.AddonError = "" })

	var zip []byte
	var targets []AddonTarget
	for _, flavor := range TargetFlavors(settings) {
		dir := wow.AddOnsDir(settings.WowDir, flavor)
		installed, ok := InstalledAddonVersion(dir)
		t := AddonTarget{Flavor: flavor, Installed: installed}
		if apply && (!ok || Newer(release.Tag, installed)) {
			if zip == nil {
				zip, err = u.gh.DownloadVerified(ctx, release, AddonAssetName(release.Tag))
			}
			if err == nil {
				err = InstallAddon(zip, dir)
			}
			if err != nil {
				t.Error = err.Error()
				log.Printf("installing addon %s into %s: %v", release.Tag, flavor, err)
			} else {
				t.Installed = release.Tag
				log.Printf("installed addon %s into %s", release.Tag, flavor)
			}
		}
		targets = append(targets, t)
	}
	u.set(func(s *Status) { s.AddonTargets = targets })
}

func (u *Updater) checkCompanion(ctx context.Context, apply bool) {
	release, err := u.gh.Latest(ctx, CompanionRepo)
	if err != nil {
		log.Printf("companion update check: %v", err)
		u.set(func(s *Status) { s.CompanionError = "Couldn't check for companion updates: " + err.Error() })
		return
	}
	u.set(func(s *Status) { s.CompanionLatest = release.Tag; s.CompanionError = "" })
	if !apply || !Newer(release.Tag, u.version) {
		return
	}
	data, err := u.gh.DownloadVerified(ctx, release, CompanionAssetName())
	if err == nil {
		err = ReplaceExecutable(data)
	}
	if err != nil {
		log.Printf("updating companion to %s: %v", release.Tag, err)
		u.set(func(s *Status) { s.CompanionError = "Update to " + release.Tag + " failed: " + err.Error() })
		return
	}
	log.Printf("updated companion %s -> %s; restarting", u.version, release.Tag)
	u.restart()
}
