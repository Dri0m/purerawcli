# GNU make, on macOS or Windows (e.g. `scoop install make` or mingw32-make).
# Recipes only call go, so they run under sh and cmd.exe alike.

PKG := ./cmd/purerawcli

ifeq ($(OS),Windows_NT)
BIN := bin/purerawcli.exe
else
BIN := bin/purerawcli
# PureRAW 6 needs macOS 13.3; target 13.0 so the binary is never the limit.
# cgo (AppKit, used to ask PureRAW to quit) would otherwise stamp the build
# host's macOS version as the minimum.
# CGO_CFLAGS, unlike MACOSX_DEPLOYMENT_TARGET alone, is part of Go's build cache
# key, so runtime objects cached from a build without it aren't reused.
export MACOSX_DEPLOYMENT_TARGET := 13.0
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=13.0
export CGO_ENABLED := 1
endif

.PHONY: build windows universal test

build:
ifneq ($(OS),Windows_NT)
	rm -f $(BIN)
endif
	go build -o $(BIN) $(PKG)

# Windows x64 binary (pure Go), buildable from macOS too. PureRAW 6 for
# Windows is x64-only.
windows: export GOOS := windows
windows: export GOARCH := amd64
windows: export CGO_ENABLED := 0
windows:
	go build -o bin/purerawcli.exe $(PKG)

# macOS only: Apple Silicon + Intel in one binary.
universal:
	rm -f bin/purerawcli
	GOARCH=arm64 CC="clang -arch arm64" go build -o bin/purerawcli-arm64 $(PKG)
	GOARCH=amd64 CC="clang -arch x86_64" go build -o bin/purerawcli-amd64 $(PKG)
	lipo -create -output bin/purerawcli bin/purerawcli-arm64 bin/purerawcli-amd64
	rm bin/purerawcli-arm64 bin/purerawcli-amd64

test:
	go vet ./...
	go test ./...
