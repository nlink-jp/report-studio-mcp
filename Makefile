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

.PHONY: build build-all package verify-release verify-skill-zip test package-skill install-skill uninstall-skill clean help

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
## README + LICENSE inside; notarize it. Also builds the skill zip
## (package-skill): every release carries both.
package: build-all package-skill
	@cd $(BIN_DIR) && stage=_pkg; rm -rf $$stage; mkdir -p $$stage; \
		cp "$(BINARY)-darwin-arm64" "$$stage/$(BINARY)"; \
		cp ../README.md ../LICENSE $$stage/; \
		( cd $$stage && zip -q "../$(BINARY)-$(VERSION)-darwin-arm64.zip" * ); \
		rm -rf $$stage
	@scripts/notarize-darwin.sh $(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip "$(NOTARY_PROFILE)"

## verify-release: refuse to release a zip that is un-notarized, stale, does
## not unpack, does not run, or holds a build from another tag. Every step
## fails closed; only the spctl line is informational. Its prerequisite
## verify-skill-zip refuses a release without the skill zip.
verify-release: verify-skill-zip
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

## test: Run all unit tests and the organization's skill validator
test:
	go test ./...
	@tests/validate-skill.sh

SKILLS_DEST ?= $(HOME)/.claude/skills
SKILL_NAME  := report-manuscript
SKILL_ZIP   := $(BIN_DIR)/$(SKILL_NAME)-$(VERSION).zip

## package-skill: Zip the bundled skill → dist/report-manuscript-<version>.zip.
## The zip's root is the skill folder — the layout the app's skill registration
## accepts and the skills-series repositories ship. A separate release asset.
package-skill:
	@tests/validate-skill.sh >/dev/null
	@mkdir -p $(BIN_DIR)
	@rm -f "$(SKILL_ZIP)"
	@zip -qrX "$(SKILL_ZIP)" "$(SKILL_NAME)" -x '*.DS_Store'
	@echo "packaged: $(SKILL_ZIP)"

## verify-skill-zip: refuse a release from a dirty tree, or whose skill zip is
## missing, holds anything but the tracked files of the skill at its root, or
## differs from them.
verify-skill-zip:
	@case "$(VERSION)" in *-dirty) \
		echo "verify-skill-zip: FAIL — $(VERSION): the working tree has uncommitted changes, so the zip would not match the tag."; \
		exit 1;; esac
	@test -f "$(SKILL_ZIP)" || { \
		echo "verify-skill-zip: FAIL — $(SKILL_ZIP) is missing. make package builds it; every release carries it."; \
		exit 1; }
	@got=$$(unzip -Z1 "$(SKILL_ZIP)" | grep -v '/$$' | LC_ALL=C sort); \
		want=$$(git ls-files "$(SKILL_NAME)" | LC_ALL=C sort); \
		if [ "$$got" != "$$want" ]; then \
			echo "verify-skill-zip: FAIL — $(SKILL_ZIP) does not hold exactly the tracked files of $(SKILL_NAME)/, at its root."; \
			echo "  An untracked file in $(SKILL_NAME)/ ends up in the zip too; remove it or commit it, then re-run make package."; \
			exit 1; fi
	@tmp=$$(mktemp -d); rc=0; \
		if unzip -q "$(SKILL_ZIP)" -d "$$tmp"; then \
			for f in $$(git ls-files "$(SKILL_NAME)"); do \
				cmp -s "$$tmp/$$f" "$$f" || { echo "verify-skill-zip: FAIL — $$f in $(SKILL_ZIP) differs from the working tree (re-run make package)."; rc=1; break; }; \
			done; \
		else echo "verify-skill-zip: FAIL — $(SKILL_ZIP) does not unpack."; rc=1; fi; \
		rm -rf "$$tmp"; \
		exit $$rc
	@echo "verify-skill-zip: OK ($(SKILL_ZIP))"

## install-skill: Install the skill from the built zip into SKILLS_DEST
## (default ~/.claude/skills). Unpacking the zip rather than copying the source
## tree means a packaging defect breaks this install instead of a release.
install-skill: package-skill
	@mkdir -p "$(SKILLS_DEST)"
	rm -rf "$(SKILLS_DEST)/$(SKILL_NAME)"
	@unzip -q "$(SKILL_ZIP)" -d "$(SKILLS_DEST)"
	@echo "installed from $(SKILL_ZIP) -> $(SKILLS_DEST)/$(SKILL_NAME)"

## uninstall-skill: Remove the installed report-manuscript skill
uninstall-skill:
	rm -rf "$(SKILLS_DEST)/$(SKILL_NAME)"
	@echo "removed: $(SKILLS_DEST)/$(SKILL_NAME)"

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
