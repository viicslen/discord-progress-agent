{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  packages = with pkgs; [
    go
    gcc
    pkg-config
    git

    # Fyne (CGO) build deps: OpenGL + X11 headers/libs.
    libGL
    libx11
    libxcursor
    libxi
    libxinerama
    libxrandr
    libxxf86vm
    libxext
  ];

  shellHook = ''
    export CGO_ENABLED=1
    # So the built binary can find libGL at runtime.
    export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath [
      pkgs.libGL
      pkgs.libx11
      pkgs.libxcursor
      pkgs.libxi
      pkgs.libxinerama
      pkgs.libxrandr
      pkgs.libxxf86vm
      pkgs.libxext
    ]}:$LD_LIBRARY_PATH"
  '';
}
