# Build environment for the Gio prototype: `nix-shell gui/shell.nix`.
#
# This uses the system's <nixpkgs>, not the flake's pinned one, on
# purpose: Gio loads the system GL driver (/run/opengl-driver) at run
# time, and that driver only loads into a binary built against the same
# glibc. A binary built from an older pin fails with
# "eglGetDisplay failed: 0x300c".
{ pkgs ? import <nixpkgs> { } }:
pkgs.mkShell {
  packages = with pkgs; [
    go pkg-config wayland libxkbcommon libGL vulkan-headers
    libx11 libxcursor libxfixes libxcb
  ];
}
