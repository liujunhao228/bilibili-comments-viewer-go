# Makefile for bilibili-comments-viewer-go

APP_NAME = bcvg
MAIN_FILE = main.go

# Detect OS
ifeq ($(OS),Windows_NT)
	EXE = .exe
else
	EXE =
endif

# Build flags
LDFLAGS = -s -w
TRIMPATH = -trimpath

.PHONY: all build clean upx

all: build

build:
	go build $(TRIMPATH) -ldflags="$(LDFLAGS)" -o $(APP_NAME)$(EXE) $(MAIN_FILE)

clean:
	rm -f $(APP_NAME)$(EXE)

# Optional: UPX compress (requires upx installed)
upx: build
	upx --best --lzma $(APP_NAME)$(EXE) 