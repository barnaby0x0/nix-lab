{
  description = "Development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forEachSystem = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      devShells = forEachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          myVim = pkgs.vim-full.customize {
            name = "vim";
            vimrcConfig.customRC = ''
              set number
              set relativenumber
              set syntax=on
              set tabstop=4
              set shiftwidth=4
              set expandtab
              colorscheme desert

              echo "Vim configuré par Nix activé !"
            '';
            # Optionnel : Vous pouvez aussi lui injecter des plugins
            vimrcConfig.packages.monPack = with pkgs.vimPlugins; {
              start = [
                vim-airline
                ale
              ];
            };
          };

        in
        {
          default = pkgs.mkShell {
            nativeBuildInputs = with pkgs; [
              # Add your development tools here, e.g.:
              git
              nodejs
              python3
            ];

            shellHook = ''
              echo "Entering development shell!"
            '';
          };
          devops = pkgs.mkShell {
            packages = with pkgs; [
              myVim
            ];

            shellHook = ''
              echo "Entering Devops shell!"
            '';
          };
        }
      );
      formatter = forEachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        pkgs.nixfmt-rfc-style
      );
    };
}
