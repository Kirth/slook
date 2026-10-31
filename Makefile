.PHONY: build install test lint run clean gui

BINARY=slk
BUILD_DIR=bin

build:
	go build -ldflags="-s -w" -trimpath -o $(BUILD_DIR)/$(BINARY) ./cmd/slk

install:
	go install -ldflags="-s -w" -trimpath ./cmd/slk

# Both modules: the root (the slk CLI and the shared internal/
# packages) and gui/ (slk-gio, its own module).
test:
	go test ./... -v -race
	cd gui && go test ./... -v -race

lint:
	golangci-lint run ./...
	cd gui && golangci-lint run ./...

# slk is a CLI (workspace setup, export); the client is slk-gio.
run: gui
	./$(BUILD_DIR)/slk-gio

# The Gio client in gui/ (its own module). With Nix it always builds
# inside gui/shell.nix, whatever shell make runs in: the flake's pinned
# nixpkgs has an older glibc than the system's graphics driver, so a
# binary built under `nix develop` cannot open a window (EGL fails, then
# "libvulkan.so.1: cannot open shared object file"). Without Nix, it
# builds with the system's libraries.
gui:
	@if command -v nix-shell >/dev/null; then \
		echo "nix-shell gui/shell.nix: go build -o $(BUILD_DIR)/slk-gio ./cmd/slk-gio"; \
		nix-shell gui/shell.nix --run 'cd gui && go build -o ../$(BUILD_DIR)/slk-gio ./cmd/slk-gio'; \
	else \
		cd gui && go build -o ../$(BUILD_DIR)/slk-gio ./cmd/slk-gio; \
	fi

clean:
	rm -rf $(BUILD_DIR)
