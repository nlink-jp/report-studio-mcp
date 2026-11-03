MODULE  := github.com/nlink-jp/report-studio-mcp
BINARY  := report-studio-mcp
BIN_DIR := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X $(MODULE)/cmd.Version=$(VERSION)"

# macOS Developer ID signing / notarization (see CONVENTIONS.md §Code
# Signing). Defaults match any Developer ID Application cert in the
# keychain and the org-standard notary profile. Builds without these
# fall back to ad-hoc / un-notarized with a one-line warning.
CODESIGN_IDENTITY ?= Developer ID Application
NOTARY_PROFILE    ?= nlink-jp-notary

# Typesetting goes through macOS Core Text and Core Graphics via cgo, so this
# tool is darwin/arm64 only: there is no cross-compiled matrix (see the RFP).
#
# The minimum macOS is pinned for every object, Go's runtime cgo objects
# included: -mmacosx-version-min in CGO flags alone leaves those built for the
# build machine's OS (phase 0, P0-1). Check with `vtool -show-build`.
export MACOSX_DEPLOYMENT_TARGET := 26.0

.PHONY: build build-all package verify-release test clean help

## build: Build binary for this Mac → ./dist/report-studio-mcp
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) .
	@scripts/codesign-darwin.sh $(BIN_DIR)/$(BINARY) "$(CODESIGN_IDENTITY)"

## build-all: Release binary (darwin/arm64 only), codesigned
build-all:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY)-darwin-arm64 .
	@scripts/codesign-darwin.sh $(BIN_DIR)/$(BINARY)-darwin-arm64 "$(CODESIGN_IDENTITY)" "$(BINARY)"

## package: <name>-v<version>-darwin-arm64.zip with the canonical binary +
## README + LICENSE inside; notarize it.
package: build-all
	@cd $(BIN_DIR) && stage=_pkg; rm -rf $$stage; mkdir -p $$stage; \
		cp "$(BINARY)-darwin-arm64" "$$stage/$(BINARY)"; \
		cp ../README.md ../LICENSE $$stage/; \
		( cd $$stage && zip -q "../$(BINARY)-$(VERSION)-darwin-arm64.zip" * ); \
		rm -rf $$stage
	@scripts/notarize-darwin.sh $(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip "$(NOTARY_PROFILE)"

## verify-release: refuse to release a zip that is un-notarized, stale, does
## not unpack, does not run, or holds a build from another tag. Every step
## fails closed; only the spctl line is informational.
verify-release:
	@test -f "$(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip.notarized" || { \
		echo "verify-release: FAIL — $(BINARY)-$(VERSION)-darwin-arm64.zip has no notarization marker."; \
		echo "  make package must end with '[notarize] ...: Accepted'. Do not upload this zip."; \
		exit 1; }
	@test "$(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip.notarized" -nt "$(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip" || { \
		echo "verify-release: FAIL — the zip was rebuilt after its marker (re-run make package)."; \
		exit 1; }
	@tmp=$$(mktemp -d); rc=0; \
		if ! unzip -oq "$(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip" -d "$$tmp"; then \
			echo "verify-release: FAIL — the zip does not unpack. Do not upload it."; rc=1; \
		elif ! out=$$("$$tmp/$(BINARY)" --version 2>&1); then \
			echo "verify-release: FAIL — the packaged binary does not run:"; \
			echo "  $$out"; rc=1; \
		elif ! printf '%s\n' "$$out" | grep -qF "$(VERSION)"; then \
			echo "verify-release: FAIL — the packaged binary reports \"$$out\", not $(VERSION)."; \
			echo "  The zip holds a build from another tag (re-run make package)."; rc=1; \
		else \
			echo "  $$out"; \
			spctl -a -vv -t install "$$tmp/$(BINARY)" 2>&1 | head -2 || true; \
		fi; \
		rm -rf "$$tmp"; \
		exit $$rc
	@echo "verify-release: OK ($(VERSION), notarized, unpacks, runs, reports its version)"

## test: Run all unit tests
test:
	go test ./...

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## help: Show available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

# Homebrew tap generation (see scripts/release-brew.mk). After `make package`,
# `make brew` generates this formula from the built darwin-arm64 zip into the
# local nlink-jp/homebrew-tap checkout. The package target is unchanged.
BREW_KIND := formula
BREW_DESC := MCP server that typesets Markdown reports into PDF and Word documents
include scripts/release-brew.mk
