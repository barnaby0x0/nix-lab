{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    nixos-lima = {
      url = "github:nixos-lima/nixos-lima";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs =
    {
      self,
      nixpkgs,
      nixos-generators,
      nixos-lima,
      ...
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      formats = [
        "raw-efi"
        "iso"
      ];

      # system = "aarch64-linux";
      forAllSystems =
        f:
        nixpkgs.lib.genAttrs systems (
          system:
          f rec {
            inherit system;
            pkgs = import nixpkgs { inherit system; };
          }
        );

      host = "lima";

      mkHost = hostname: system: {
        name = "${hostname}-${system}";
        value = nixpkgs.lib.nixosSystem {
          inherit system;
          modules = [
            ./configuration.nix
            nixos-lima.nixosModules.lima
          ];
        };
      };

    in
    {
      # packages = forAllSystems (
      #   { system, ... }: {
      #     image = nixos-generators.nixosGenerate {
      #       inherit system;
      #       modules = [
      #         nixos-lima.nixosModules.lima
      #         ./configuration.nix
      #       ];
      #       format = "raw-efi";
      #     };
      #   }
      # );

      # packages.${system}.image = nixos-generators.nixosGenerate {
      #   inherit system;

      #   modules = [
      #     # Charge le module requis qui définit "services.lima"
      #     nixos-lima.nixosModules.lima
      #     ./configuration.nix
      #   ];
      #   format = "raw-efi";
      # };

      # FORME 1
      # nixosConfigurations = {
      #   lima = nixpkgs.lib.nixosSystem {
      #     system = "aarch64-linux";
      #     modules = [
      #       ./configuration.nix
      #       nixos-lima.nixosModules.lima
      #       #./hardware-configuration.nix
      #     ];
      #   };
      # };

      # FORME 2
      # nixosConfigurations = nixpkgs.lib.genAttrs (map (system: "${host}-${system}") systems) (
      #   name:
      #   let
      #     system = if nixpkgs.lib.hasSuffix "aarch64-linux" name then "aarch64-linux" else "x86_64-linux";
      #   in
      #   nixpkgs.lib.nixosSystem {
      #     inherit system;
      #     modules = [
      #       ./configuration.nix
      #       nixos-lima.nixosModules.lima
      #     ];
      #   }
      # );

      # FORME 3
      # nixosConfigurations = builtins.listToAttrs (
      #   map (system: {
      #     name = "${host}-${system}";
      #     value = nixpkgs.lib.nixosSystem {
      #       inherit system;
      #       modules = [
      #         ./configuration.nix
      #         nixos-lima.nixosModules.lima
      #       ];
      #     };
      #   }) systems
      # );

      # FORME 4
      # nixosConfigurations = nixpkgs.lib.genAttrs (map (system: "${host}-${system}") systems) (
      #   name:
      #   let
      #     system = builtins.elemAt systems (
      #       nixpkgs.lib.lists.findFirstIndex (system: "${host}-${system}" == name) 0 systems
      #     );
      #   in
      #   nixpkgs.lib.nixosSystem {
      #     inherit system;
      #     modules = [
      #       ./configuration.nix
      #       nixos-lima.nixosModules.lima
      #     ];
      #   }
      # );

      # FORME 5
      nixosConfigurations = builtins.listToAttrs (map (system: mkHost "lima" system) systems);

      # packages = forAllSystems (
      #   { system, ... }: {
      #     image = self.nixosConfigurations."lima-${system}".config.system.build.images."raw-efi";
      #   }
      # );
      # Avec gestion des formats
      packages = forAllSystems (
        { system, ... }:
        builtins.listToAttrs (
          map (format: {
            name = "image-${format}";
            value = self.nixosConfigurations."lima-${system}".config.system.build.images.${format};
          }) formats
        )
      );

      formatter = forAllSystems ({ pkgs, ... }: pkgs.nixfmt);
    };
}
