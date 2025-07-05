# Makefile for bilibili-comments-viewer-go

APP_NAME = bcvg
MAIN_FILE = main.go
CONFIG = config.yaml
README = README.md

# Detect OS
ifeq ($(OS),Windows_NT)
	EXE = .exe
else
	EXE =
endif

# Build flags
LDFLAGS = -s -w
TRIMPATH = -trimpath

# 多平台参数
PLATFORMS = \
	"windows amd64 .exe bcvg-windows-amd64.zip" \
	"linux amd64 '' bcvg-linux-amd64.tar.gz" \
	"darwin amd64 '' bcvg-darwin-amd64.tar.gz"

# Artifact policy: clean, dist, none
ARTIFACT_POLICY ?= dist
DIST_DIR = dist

.PHONY: all build clean upx pack release

all: clean build pack

build:
	go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(APP_NAME)$(EXE) $(MAIN_FILE)

clean:
	rm -f $(APP_NAME) $(APP_NAME).exe bcvg-*.zip bcvg-*.tar.gz

# Optional: UPX compress (requires upx installed)
upx: build
	upx --best --lzma $(APP_NAME)$(EXE)

pack:
	@if [ "$(ARTIFACT_POLICY)" = "dist" ]; then \
		mkdir -p $(DIST_DIR); \
	fi; \
	set -e; \
	for entry in $(PLATFORMS); do \
		set -- $$entry; \
		GOOS=$$1; GOARCH=$$2; EXT=$$3; PKG=$$4; \
		OUT=$(APP_NAME)$$EXT; \
		echo "Building $$OUT for $$GOOS/$$GOARCH..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $$OUT $(MAIN_FILE); \
		FILES="$$OUT $(CONFIG) $(README)"; \
		for f in $$OUT $(CONFIG) $(README); do \
			[ -f $$f ] || { echo "Missing file: $$f"; exit 1; }; \
		done; \
		echo "Packing $$PKG..."; \
		if echo $$PKG | grep -q '.zip'; then \
			7z a -tzip $$PKG $$OUT $(CONFIG) $(README); \
		else \
			tar czf $$PKG $$OUT $(CONFIG) $(README); \
		fi; \
		if [ "$(ARTIFACT_POLICY)" = "dist" ]; then \
			mv $$PKG $(DIST_DIR)/; \
		fi; \
		rm -f $$OUT; \
	done

release: pack
	@TAG=${TAG}; \
	if [ -z "$$TAG" ]; then \
		TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v$$(date +%Y%m%d%H%M%S)"); \
	fi; \
	if ! git rev-parse "$$TAG" >/dev/null 2>&1; then \
		echo "No tag found, creating tag $$TAG..."; \
		git tag "$$TAG"; \
		git push origin "$$TAG"; \
	fi; \
	if gh release view "$$TAG" >/dev/null 2>&1; then \
		echo "Release $$TAG already exists, deleting..."; \
		gh release delete "$$TAG" -y; \
	fi; \
	if [ -f CHANGELOG.md ]; then \
		NOTES=$$(awk "/^## $$TAG/,/^## v/" CHANGELOG.md | sed '1d;$$d'); \
	else \
		NOTES="Auto release by Makefile."; \
	fi; \
	echo "Creating GitHub release $$TAG..."; \
	if [ "$(ARTIFACT_POLICY)" = "dist" ]; then \
		gh release create "$$TAG" $(DIST_DIR)/bcvg-*.zip $(DIST_DIR)/bcvg-*.tar.gz --title "$$TAG" --notes "$$NOTES" || { echo "GitHub 发布失败"; exit 1; }; \
	else \
		gh release create "$$TAG" bcvg-*.zip bcvg-*.tar.gz --title "$$TAG" --notes "$$NOTES" || { echo "GitHub 发布失败"; exit 1; }; \
	fi; \
	if [ "$(ARTIFACT_POLICY)" = "clean" ]; then \
		rm -f bcvg-*.zip bcvg-*.tar.gz; \
	elif [ "$(ARTIFACT_POLICY)" = "dist" ]; then \
		rm -rf $(DIST_DIR); \
	fi 