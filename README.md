# GuildLink companion

A small background app that watches the GuildLink addon's SavedVariables and uploads them to the GuildLink Discord bot whenever the game writes a new version (on logout or `/reload`). It runs on Windows, macOS and Linux, with a tray icon and a settings page at <http://127.0.0.1:47631/>.

## Use

1. Run `/link` in Discord to get the server address and token.
2. Start the companion. The settings page opens in your browser. Paste the address and token, pick your World of Warcraft folder (usual install locations are detected, including Lutris, Wine, Steam/Proton and Bottles prefixes on Linux), then **Save** and **Test connection**.
3. Play. The status page shows every account's saved data it found and the result of the last upload.

It finds `GuildLink.lua` under every game flavor and account (`<WoW>/_*_/WTF/Account/*/SavedVariables/GuildLink.lua`) and uploads each one when it changes. If an upload fails, it retries a minute later.

With **Keep addon data between sessions** on (the default), it also copies the newest saved data into the addon's `Seed.lua`. This works around the Forever beta client not loading SavedVariables (see the addon README).

Flags: `-headless` (no tray icon, stop with Ctrl+C), `-no-browser`, `-config <file>`, `-version`.

Settings are stored with owner-only permissions in the user config folder (`%AppData%\GuildLink`, `~/Library/Application Support/GuildLink`, `~/.config/GuildLink`), next to `companion.log`. The settings page only accepts requests from itself: it checks the Host header and a per-run CSRF token.

## Build

Requires Go 1.24+.

```sh
make test
make windows linux   # pure Go, cross-compiles from any OS
make darwin          # on a Mac (the tray icon uses cgo); elsewhere: make darwin-notray
```

Binaries go to `dist/` as `guildlink-companion-<os>-<arch>` for amd64 and arm64. Pushing a `v*` tag makes GitHub Actions build all six and attach them to a release. macOS builds run on a macOS runner.

Unsigned binaries trigger Gatekeeper and SmartScreen warnings. Sign and notarize them before handing them out widely.

## Layout

```
cmd/guildlink-companion   entrypoint, tray icon
internal/luasv            SavedVariables (Lua table) parser
internal/addon            addon data → upload payload
internal/wow              install detection, file discovery, Seed.lua
internal/syncer           polling and upload loop
internal/web              local settings/status page
internal/api              bot API client
```
