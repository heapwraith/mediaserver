# mediaserver — developer commands (AGENTS.md §10).
# Requires: Go 1.27, Node 24+, make and bash (on Windows: run from Git Bash).
# golangci-lint is pinned in tools.mod.

# "bash" (not /bin/bash) so Windows make finds Git Bash in the PATH.
SHELL        := bash
.SHELLFLAGS  := -eu -o pipefail -c
.DEFAULT_GOAL := help

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
GO        ?= go
NPM       := npm --prefix web
LINT      := $(GO) tool -modfile=tools.mod golangci-lint
DIST      := dist
EXE       := $(shell $(GO) env GOEXE)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# make tv-* (docs/tv/): packages go to dist/tv
TV_DIST       := $(DIST)/tv
TIZEN_PROFILE ?= mediaserver

# make dev
MEDIA_DIR ?= $(CURDIR)/media
DEV_DIR   := .dev

.PHONY: help dev build build-web build-go test fmt check bench load clean web-deps web-stub tv tv-tizen tv-webos tv-android

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

# --- setup -----------------------------------------------------------------

web/node_modules/.package-lock.json: web/package.json web/package-lock.json
	$(NPM) ci --no-fund --no-audit
	@touch $@

web-deps: web/node_modules/.package-lock.json

# The Go binary embeds web/build. Before the first frontend build, an empty
# placeholder keeps `go build`, `go test` and the linter working.
web-stub:
	@mkdir -p web/build
	@[ -n "$$(ls -A web/build)" ] || touch web/build/.gitkeep

# --- main targets ----------------------------------------------------------

dev: web-deps web-stub ## Backend with reload (Go) + Vite dev server on :5173
	@mkdir -p "$(MEDIA_DIR)" $(DEV_DIR)
	@echo "MEDIA_DIR=$(MEDIA_DIR) · API :8080 · frontend http://localhost:5173"
	@trap 'trap - EXIT INT TERM; kill 0' EXIT INT TERM; \
	$(NPM) run dev & \
	while true; do \
		touch $(DEV_DIR)/stamp; \
		if $(GO) build -o $(DEV_DIR)/mediaserver$(EXE) ./cmd/mediaserver; then \
			APP_ENV=dev MEDIA_DIR="$(MEDIA_DIR)" DATA_DIR=$(DEV_DIR)/data $(DEV_DIR)/mediaserver$(EXE) & pid=$$!; \
		else pid=; fi; \
		until [ -n "$$(find cmd internal migrations web/embed.go -newer $(DEV_DIR)/stamp \( -name '*.go' -o -name '*.sql' \) 2>/dev/null | head -1)" ]; do sleep 1; done; \
		echo "--- change detected, restarting backend"; \
		[ -z "$$pid" ] || { kill -INT $$pid; wait $$pid || true; }; \
	done

build: build-web build-go ## Frontend + 6 binaries (windows/linux/darwin × amd64/arm64), no CGO

build-web: web-deps
	$(NPM) run build

build-go: web-stub
	@mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		out=$(DIST)/mediaserver-$$os-$$arch$$ext; \
		echo "  $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/mediaserver; \
	done

# -race needs CGO and a C compiler (gcc/clang). It is only used by the tests;
# release binaries stay CGO-free. Without a C compiler the tests run without -race.
test: web-deps web-stub ## go test -race + vitest
	@if command -v "$$($(GO) env CC)" >/dev/null 2>&1; then \
		echo "CGO_ENABLED=1 $(GO) test -race ./..."; \
		CGO_ENABLED=1 $(GO) test -race ./...; \
	else \
		echo "warning: no C compiler ($$($(GO) env CC)) found: running tests WITHOUT -race (install gcc or clang to enable it)"; \
		CGO_ENABLED=0 $(GO) test ./...; \
	fi
	$(NPM) test

fmt: web-deps ## golangci-lint fmt + oxfmt
	$(LINT) fmt
	$(NPM) run fmt

check: web-deps web-stub ## golangci-lint run + oxlint + oxfmt --check + svelte-check + build budget/CSS check
	$(LINT) fmt --diff
	$(LINT) run
	$(NPM) run check
	$(NPM) run check:build

bench: web-stub ## Benchmarks (scanner and stream, from Sprint 1 on)
	$(GO) test -run='^$$' -bench=. -benchmem ./...

load: web-stub ## Load test: 2 direct plays + 1 conversion at once (conversion needs FFmpeg)
	MEDIASERVER_LOAD=1 $(GO) test -run='^TestLoad$$' -count=1 -v ./internal/api

# --- TV apps (docs/tv/) ----------------------------------------------------
# Each package holds only the connection page (web/build-tv); the interface
# comes from the server (docs/adr/008-tv-apps.md).

tv: web-deps ## TV apps' connection page (web/build-tv)
	$(NPM) run build:tv

tv-tizen: tv ## Samsung .wgt (Tizen Studio CLI + certificate profile TIZEN_PROFILE)
	@command -v tizen >/dev/null || { echo "tizen CLI not found: install Tizen Studio (docs/tv/samsung.md)"; exit 1; }
	rm -rf $(TV_DIST)/tizen && mkdir -p $(TV_DIST)/tizen
	cp tv/tizen/config.xml tv/tizen/icon.png web/build-tv/index.html $(TV_DIST)/tizen/
	cd $(TV_DIST)/tizen && tizen package -t wgt -s $(TIZEN_PROFILE) -- .
	@ls $(TV_DIST)/tizen/*.wgt

tv-webos: tv ## LG .ipk (webOS CLI: npm i -g @webos-tools/cli)
	@command -v ares-package >/dev/null || { echo "ares-package not found: npm i -g @webos-tools/cli (docs/tv/lg.md)"; exit 1; }
	rm -rf $(TV_DIST)/webos && mkdir -p $(TV_DIST)/webos/app
	cp tv/webos/appinfo.json tv/webos/icon.png tv/webos/largeIcon.png web/build-tv/index.html $(TV_DIST)/webos/app/
	ares-package $(TV_DIST)/webos/app -o $(TV_DIST)/webos
	@ls $(TV_DIST)/webos/*.ipk

tv-android: tv ## Android TV / Google TV / Fire TV .apk (Android SDK, ANDROID_HOME)
	cd tv/android && MEDIASERVER_VERSION=$(VERSION) ./gradlew --no-daemon -q assembleRelease
	mkdir -p $(TV_DIST)
	cp tv/android/app/build/outputs/apk/release/app-release.apk $(TV_DIST)/mediaserver-tv.apk
	@ls $(TV_DIST)/mediaserver-tv.apk

clean: ## Remove build outputs
	rm -rf $(DIST) $(DEV_DIR) web/build web/build-tv web/.svelte-kit tv/android/build tv/android/app/build
