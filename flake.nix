{
  description = "Slack CLI and Gio desktop client";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
        lib = pkgs.lib;
        slk = pkgs.buildGo126Module {
          pname = "slk";
          version = "0.0.0";
          src = ./.;
          vendorHash = "sha256-3zdIahgS0Z8hcuUO/Z6YR5MhTIruhXJwDKd1KUMJJH4=";
          # gui/ is its own module (slk-gio); buildGoModule's package
          # search would otherwise try to build it from this one.
          excludedPackages = [ "gui" ];
          # The test suite spins up httptest servers on loopback. The macOS
          # Nix sandbox denies all networking by default, so allow loopback
          # there; the attribute is a no-op on Linux.
          __darwinAllowLocalNetworking = true;
        };
      in {
        packages.default = slk;
        packages.slk = slk;
        # `nix develop`: the slk CLI's build inputs (Go). slk-gio is not
        # packaged here: gui/shell.nix builds it against the system's
        # nixpkgs so it links the same glibc as the graphics driver.
        devShells.default = pkgs.mkShell {
          inputsFrom = [ slk ];
        };
      });
}
