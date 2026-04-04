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
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go
            go
            gopls
            gotools
            go-tools # staticcheck

            # Protobuf
            buf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc

            # Node.js (frontend)
            nodejs_22
            nodePackages.npm

            # SQLite (for CLI debugging)
            sqlite

            # Misc
            gnumake
          ];

          shellHook = ''
            export GOPATH="$PWD/.go"
            export PATH="$GOPATH/bin:$PATH"
          '';
        };
      });
}
