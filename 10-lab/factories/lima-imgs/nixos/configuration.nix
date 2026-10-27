{ config, pkgs, ... }: {

  fileSystems."/" = {
    device = "/dev/disk/by-label/nixos";
    fsType = "ext4";
  };

  boot.loader.grub.enable = false;

  services.lima.enable = true;

  environment.systemPackages = with pkgs; [
    cowsay
    git
    gnumake
    gnupg
    lolcat
    vim
  ];

  programs.gnupg.agent = {
    enable = true;
    enableSSHSupport = true; # Optionnel : remplace ssh-agent par gpg-agent
    pinentryPackage = pkgs.pinentry-curses; # ou pkgs.pinentry-gnome3, pkgs.pinentry-qt, etc.
  };

  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "yes";
      PasswordAuthentication = false;
    };
  };

  users.users.lima = {
    isNormalUser = true;
    extraGroups = [
      "wheel"
      "docker"
    ];
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDitkz551tlK4b/Xwa8yCJQ3xJ79L3AGRUcPf57n+mxf vbeauvoir@vbeauvoir-MacBook-H2ND75YK9T.local"
    ];
  };

  security.sudo.wheelNeedsPassword = false;

  security.pki.certificateFiles = [
    ./root-ca.crt
  ];

  nix.settings.experimental-features = [
    "nix-command"
    "flakes"
  ];

  system.stateVersion = "26.05";
}
