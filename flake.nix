{
  description = "Backup Orchestrator dev environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        air = pkgs.buildGoModule rec {
          pname = "air";
          version = "1.65.0";
          src = pkgs.fetchFromGitHub {
            owner = "air-verse";
            repo = "air";
            rev = "v${version}";
            hash = "sha256-pqvnX/PiipZM8jLBN6zN/yVnuCCk+aTII5AH0N4nHEM=";
          };
          vendorHash = "sha256-03xZ3P/7xjznYdM9rv+8ZYftQlnjJ6ZTq0HdSvGpaWw=";
          subPackages = [ "." ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go
            go_1_26
            gopls
            gotools
            go-tools # staticcheck

            # Protobuf
            buf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc

            # Node.js (frontend)
            nodejs_24

            # SQLite (for CLI debugging)
            sqlite

            # Linting
            golangci-lint

            # Task runner + pre-commit hooks
            just
            lefthook

            # Misc
            gnumake
            rclone
            restic
            gh

            # Dev tooling
            zellij
            air
          ];

          shellHook = ''
            export GOPATH="$PWD/.go"
            export PATH="$GOPATH/bin:$PATH"
          '';
        };
      });
}
