.PHONY: build install test lint run clean demo-gifs gui

BINARY=slk
BUILD_DIR=bin

build:
	go build -ldflags="-s -w" -trimpath -o $(BUILD_DIR)/$(BINARY) ./cmd/slk

install:
	go install -ldflags="-s -w" -trimpath ./cmd/slk

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

run: build
	./$(BUILD_DIR)/$(BINARY)

# The Gio prototype in gui/ (its own module). With Nix it always builds
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

# Renders docs/assets/demo/*.gif from their VHS tapes (settings.tape is
# shared, not rendered). Needs vhs: run inside `nix develop`, or see
# https://github.com/charmbracelet/vhs. Not run in CI.
demo-gifs: build
	@command -v vhs >/dev/null || { echo "vhs not found: run 'nix develop' or install charmbracelet/vhs"; exit 1; }
	@for tape in docs/assets/demo/*.tape; do \
		case $$tape in */settings.tape) continue;; esac; \
		echo "vhs $$tape"; vhs $$tape || exit 1; \
	done
