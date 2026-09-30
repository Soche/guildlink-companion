# Builds release binaries into dist/.
#
# Windows and Linux builds are pure Go (CGO_ENABLED=0), so they cross-compile
# from any machine. The macOS tray icon needs cgo, so the darwin targets must
# be built on a Mac with the Xcode command line tools; elsewhere, use
# `make darwin-notray` for macOS builds without the tray icon.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PKG := ./cmd/guildlink-companion
OUT := dist
NAME := guildlink-companion

.PHONY: all windows linux darwin darwin-notray test clean

all: windows linux darwin

test:
	go vet ./...
	go test ./...

windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS) -H windowsgui" -o $(OUT)/$(NAME)-windows-amd64.exe $(PKG)
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS) -H windowsgui" -o $(OUT)/$(NAME)-windows-arm64.exe $(PKG)

linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT)/$(NAME)-linux-amd64 $(PKG)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT)/$(NAME)-linux-arm64 $(PKG)

darwin:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT)/$(NAME)-darwin-arm64 $(PKG)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT)/$(NAME)-darwin-amd64 $(PKG)

darwin-notray:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT)/$(NAME)-darwin-arm64 $(PKG)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT)/$(NAME)-darwin-amd64 $(PKG)

clean:
	rm -rf $(OUT)
