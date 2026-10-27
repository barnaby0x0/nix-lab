# Génération d'une Image NixOS pour Lima (sur macOS Apple Silicon)

Ce guide explique comment compiler une image NixOS personnalisée depuis un environnement virtuel (Colima/Ubuntu) et la déployer de manière ultra-légère sur macOS via **Lima** en utilisant le framework natif Apple Virtualization (`vz`).

---

## 🛠 Prérequis

1. Un **Mac Apple Silicon** (M1/M2/M3/M4).
2. **Colima** installé sur macOS pour l'environnement de build.
3. **Lima** installé sur macOS (`brew install lima`) pour exécuter la VM NixOS finale.

---

## Étape 1 : Configurer l'environnement de build (Colima)

NixOS a besoin des extensions de virtualisation matérielle (**KVM**) pour compiler l'image finale. Vous devez activer la virtualisation imbriquée sur votre instance Colima.

```bash
# Arrêter l'instance actuelle
colima stop

# Relancer Colima avec le support de la virtualisation imbriquée
colima start --nested-virtualization
```

---

## Étape 2 : Configuration du Projet (Flake & NixOS)

Dans votre dossier de travail à l'intérieur de la VM Colima, créez les deux fichiers suivants :

### 1. `flake.nix`
Ce fichier déclare les dépendances (`nixos-generators` et `nixos-lima`) et configure la cible pour l'architecture ARM64 de votre Mac.

```nix
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

  outputs = { self, nixpkgs, nixos-generators, nixos-lima, ... }: 
    let
      system = "aarch64-linux";
    in
    {
      packages.\${system}.image = nixos-generators.nixosGenerate {
        inherit system; 
        modules = [
          nixos-lima.nixosModules.lima
          ./configuration.nix
        ];
        format = "raw-efi";
      };
    };
}
```

### 2. `configuration.nix`
Ce fichier configure le système NixOS embarqué. **Important :** Pensez à y ajouter votre clé publique SSH macOS.

```nix
{ config, pkgs, ... }: {
  # Active l'intégration native avec Lima
  services.lima.enable = true;

  # Configuration du serveur OpenSSH
  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "yes";
      PasswordAuthentication = false; # Attention : valeur booléenne requise (pas de chaîne "no")
    };
  };

  # Utilisateur requis par Lima pour la connexion automatique
  users.users.lima = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA...votre_cle_publique_macOS..."
    ];
  };

  # Autorise sudo sans mot de passe pour l'agent Lima
  security.sudo.wheelNeedsPassword = false;

  system.stateVersion = "24.11"; 
}
```

---

## Étape 3 : Compilation de l'image (dans la VM Colima)

Nix suit uniquement les fichiers indexés par Git. Vous devez initialiser un dépôt avant de lancer le build.

```bash
# Initialiser git et ajouter les fichiers
git init
git add flake.nix configuration.nix

# Lancer la compilation de l'image
nix build .#image --option system-features "benchmark big-parallel nixos-test uid-range kvm"
```

Une fois le build terminé, le fichier image se trouve dans `result/nixos.img`. Copiez-le vers un dossier partagé accessible par votre hôte macOS (ex: `/home/user/workspace/gen-img/nixos.img`).

---

## Étape 4 : Déploiement dans Lima (sur macOS)

Retournez sur le terminal de votre **macOS**.

### 1. Créer le fichier `nixos.yaml`
Créez un fichier de définition pour Lima en spécifiant le chemin absolu de l'image `.img` copiée :

```yaml
# nixos.yaml
images:
  - location: "/Chemin/Absolu/Vers/Votre/Dossier/nixos.img"
    arch: "aarch64"

# Utilise le framework de virtualisation natif d'Apple
vmType: "vz"

mounts:
  - location: "~"
    writable: true

ssh:
  localPort: 60022
  loadDotSSHPubKeys: true
```

### 2. Nettoyer et démarrer la VM
Si vous aviez une ancienne instance défectueuse, forcez sa suppression avant de lancer la nouvelle :

```bash
# Nettoyage strict (si nécessaire)
limactl stop --force nixos
limactl delete nixos

# Démarrer la nouvelle VM NixOS
limactl start nixos.yaml
```

---

## Étape 5 : Utilisation

Une fois le démarrage validé par Lima, connectez-vous instantanément à votre environnement NixOS personnalisé :

```bash
limactl shell nixos
```

