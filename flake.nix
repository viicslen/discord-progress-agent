{
  description = "Single-user desktop work-session tracker that posts to Discord";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        lib = pkgs.lib;

        # Bumped automatically by release-please (see release-please-config.json).
        version = "0.4.0"; # x-release-please-version

        # Fyne (CGO) runtime/build deps.
        guiDeps = with pkgs; [
          libGL
          libx11
          libxcursor
          libxi
          libxinerama
          libxrandr
          libxxf86vm
          libxext
        ];

        desktopItem = pkgs.makeDesktopItem {
          name = "discord-progress-agent";
          desktopName = "Session Agent";
          comment = "Single-user desktop work-session tracker that posts to Discord";
          exec = "session-agent";
          icon = "discord-progress-agent";
          categories = [ "Utility" "Network" ];
          startupNotify = false;
        };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "discord-progress-agent";
          inherit version;
          src = self;

          # `go mod` vendor hash — update when go.sum changes:
          #   nix build .#default 2>&1 | grep got:
          vendorHash = "sha256-GHSKdexNApbksqK672dyaeNTRQp1eOsV06FRSQKYhm4=";

          subPackages = [ "cmd/session-agent" ];

          nativeBuildInputs = [ pkgs.pkg-config ]
            ++ lib.optional pkgs.stdenv.isLinux pkgs.copyDesktopItems;
          buildInputs = guiDeps;

          # copyDesktopItems installs these into $out/share/applications.
          desktopItems = lib.optionals pkgs.stdenv.isLinux [ desktopItem ];

          # Install the icon into the hicolor theme so the .desktop entry (and
          # desktop environments) resolve it by name.
          postInstall = lib.optionalString pkgs.stdenv.isLinux ''
            install -Dm644 assets/icon.svg \
              "$out/share/icons/hicolor/scalable/apps/discord-progress-agent.svg"
          '';

          ldflags = [
            "-s"
            "-w"
            "-X"
            "discord-tracker-agent/internal/settings.Version=v${version}"
          ];

          meta = with pkgs.lib; {
            description = "Single-user desktop work-session tracker that posts to Discord";
            homepage = "https://github.com/viicslen/discord-progress-agent";
            mainProgram = "session-agent";
            platforms = platforms.unix;
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [ go gcc pkg-config git ] ++ guiDeps;
          shellHook = ''
            export CGO_ENABLED=1
            export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath guiDeps}:$LD_LIBRARY_PATH"
          '';
        };
      });
}
