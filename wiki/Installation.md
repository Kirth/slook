# Installation

slk is two programs: **`slk`**, the CLI that signs workspaces in and exports
channels, and **`slk-gio`**, the desktop client. Release packages carry the
CLI only; `slk-gio` is built from source (below).

## The `slk` CLI

Grab a prebuilt binary from the [latest release](https://github.com/gammons/slk/releases/latest), or use one of the methods below.

The shell snippets resolve the latest version automatically.

## Linux

### Debian / Ubuntu

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/gammons/slk/releases/latest | grep -oE '"tag_name": *"v[^"]+"' | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/^v//')
curl -fsSLO "https://github.com/gammons/slk/releases/latest/download/slk_${VERSION}_linux_amd64.deb"
sudo dpkg -i "slk_${VERSION}_linux_amd64.deb"
```

### Fedora / RHEL

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/gammons/slk/releases/latest | grep -oE '"tag_name": *"v[^"]+"' | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/^v//')
sudo rpm -i "https://github.com/gammons/slk/releases/latest/download/slk_${VERSION}_linux_amd64.rpm"
```

### Alpine

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/gammons/slk/releases/latest | grep -oE '"tag_name": *"v[^"]+"' | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/^v//')
curl -fsSLO "https://github.com/gammons/slk/releases/latest/download/slk_${VERSION}_linux_amd64.apk"
sudo apk add --allow-untrusted "slk_${VERSION}_linux_amd64.apk"
```

### Arch Linux

slk is available in the AUR as [`slk`](https://aur.archlinux.org/packages/slk)
(community-maintained; builds from source):

```bash
yay -S slk    # or: paru -S slk
```

### Tarball (any distro)

Swap `x86_64` for `arm64` on ARM:

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/gammons/slk/releases/latest | grep -oE '"tag_name": *"v[^"]+"' | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/^v//')
curl -fsSL "https://github.com/gammons/slk/releases/latest/download/slk_${VERSION}_linux_x86_64.tar.gz" | tar xz
sudo mv slk /usr/local/bin/
```

## macOS

### Homebrew (macOS and Linux)

```bash
brew install --cask gammons/tap/slk
```

The cask carries per-OS/arch tarballs and serves both macOS and Linux.
Anyone who installed via the old pre-v0.13.0 formula is migrated to the
cask automatically on `brew upgrade` (the formula was removed from the tap
and a `tap_migrations.json` entry added); if that ever fails, run
`brew uninstall slk && brew install --cask gammons/tap/slk` once.

### Tarball

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/gammons/slk/releases/latest | grep -oE '"tag_name": *"v[^"]+"' | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/^v//')
# Apple Silicon
curl -fsSL "https://github.com/gammons/slk/releases/latest/download/slk_${VERSION}_darwin_arm64.tar.gz" | tar xz
# Intel
curl -fsSL "https://github.com/gammons/slk/releases/latest/download/slk_${VERSION}_darwin_x86_64.tar.gz" | tar xz

sudo mv slk /usr/local/bin/
```

## Windows

Download the `windows_x86_64.zip` from the [latest release](https://github.com/gammons/slk/releases/latest), extract `slk.exe`, and add it to your `PATH`.

## Go

```bash
go install github.com/gammons/slk/cmd/slk@latest
```

## Build from source

Requires Go 1.26.

```bash
git clone https://github.com/gammons/slk.git
cd slk
make build       # bin/slk, the CLI (pure Go, no cgo)
make gui         # bin/slk-gio, the desktop client
```

## `slk-gio`

`slk-gio` needs cgo, a C compiler, `pkg-config` and Gio's libraries
(Wayland, X11, xkbcommon, EGL/GLES, Vulkan headers). It is developed and
tested on Linux.

- **Debian/Ubuntu** (the list CI uses):
  ```bash
  sudo apt-get install -y gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
    libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev \
    libxcursor-dev libvulkan-dev
  ```
- **Nix / NixOS:** `make gui` builds inside `gui/shell.nix` when `nix-shell`
  is there (Go, wayland, libxkbcommon, libGL, vulkan-headers, libX11,
  libXcursor, libXfixes, libxcb). The shell uses your system's `<nixpkgs>`
  rather than the flake's pin on purpose: Gio loads the system's GL driver,
  which only loads into a binary built against the same glibc. The flake
  therefore packages only the CLI.
- **Other distributions:** the same libraries under their names; see
  [Gio's Linux notes](https://gioui.org/doc/install/linux).

At run time slk-gio uses these when they are installed:

| Tool | For |
|---|---|
| `wl-paste` (wl-clipboard) on Wayland, `xclip` on X11 | Ctrl+V of a picture (Gio's own clipboard is text-only) and of a file path |
| `zenity`, else `kdialog`, else `yad` | the paperclip's file chooser |
| `xdg-open` | links and files |
| `flux-preview` | previewing pictures and files (else the default application) |
| a StatusNotifierItem host (KDE; GNOME with the AppIndicator extension) | the tray icon |

On macOS there is no clipboard picture reader yet (`pngpaste` would be the
tool).

Try it without a Slack account: `bin/slk-gio -fake`.

## Verify your download

```bash
curl -fsSLO https://github.com/gammons/slk/releases/latest/download/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

## Next steps

After installation, head to **[[Setup]]** to add your first workspace with `slk --add-workspace`, then start `slk-gio`.
