{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    devshell.url = "github:numtide/devshell";
  };

  outputs = { nixpkgs, flake-utils, devshell, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ devshell.overlay ];
        };
      in {
        devShell = pkgs.devshell.mkShell {
          packages = [
            pkgs.go_1_17
            pkgs.goimports
            pkgs.sqlite
          ];
        };
      });
}
