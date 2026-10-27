# Guide d'Optimisation NixOS pour préserver la durée de vie d'un SSD

Par défaut, l'architecture immuable de NixOS génère un volume d'écriture plus élevé que les distributions traditionnelles (duplication des dépendances dans le `/nix/store`, recréation d'environnements, liens symboliques). 

Ce guide rassemble les meilleures configurations pour **réduire drastiquement les cycles d'écriture (TBW)** et protéger votre SSD.

---

## 1. Déplacer les builds et téléchargements temporaires en RAM (`tmpfs`)

Lors des phases de construction (`nixos-rebuild`), NixOS écrit des gigaoctets de données temporaires avant de copier le résultat final dans le store. En déplaçant `/tmp` en mémoire vive, ces écritures n'atteignent jamais le SSD.

Ajoutez ceci à votre `/etc/nixos/configuration.nix` :

```nix
{ config, pkgs, ... }: {

  # Monte le dossier /tmp en RAM
  boot.tmp.useTmpfs = true;
  
  # Alloue 50% de la RAM maximum à /tmp (ajustez selon votre configuration)
  boot.tmp.tmpfsSize = "50%"; 

  # Force Nix à effectuer ses builds de paquets dans /tmp (donc en RAM)
  nix.settings.build-dir = "/tmp";
}
```

> ⚠️ **Attention :** Si vous avez moins de 16 Go de RAM et que vous compilez manuellement de très gros projets (ex: un navigateur web complet depuis les sources), le build peut échouer par manque d'espace. Si cela arrive, commentez la ligne `nix.settings.build-dir = "/tmp";`.

---

## 2. Optimisation du Store et gestion du Garbage Collector (GC)

Le nettoyage permanent ou trop fréquent du store peut provoquer des pics d'activité I/O (lecture et suppression massives). Il est préférable d'optimiser l'espace via le *hardlinking* et d'espacer le Garbage Collector.

```nix
{
  # Fusionne automatiquement les fichiers 100% identiques dans le store (Hardlinking)
  # Cela économise énormément d'espace disque et évite des écritures redondantes
  nix.settings.auto-optimise-store = true;

  # Automatisation et espacement du Garbage Collector
  nix.gc = {
    automatic = true;
    dates = "weekly";                    # Une fois par semaine suffit amplement
    options = "--delete-older-than 14d"; # Supprime les générations de plus de 14 jours
  };
}
```

---

## 3. Activation du service de TRIM (Maintenance du SSD)

Le TRIM permet au système d'exploitation de signaler au contrôleur du SSD quels blocs de données ne sont plus utilisés, afin qu'il puisse les effacer efficacement en arrière-plan. Cela maintient les performances et prolonge la vie des cellules flash.

```nix
{
  # Active le TRIM hebdomadaire automatique
  services.fstrim.enable = true;
}
```

### Option alternative pour Btrfs / Ext4 (Options de montage)
Si vous utilisez **Btrfs**, il est recommandé d'activer le TRIM asynchrone continu et de désactiver l'écriture de l'heure de dernier accès aux fichiers (`noatime`) dans vos options de montage de partition :

```nix
fileSystems."/" = {
  device = "/dev/disk/by-uuid/votre-uuid";
  fsType = "btrfs";
  options = [ "subvol=root" "noatime" "discard=async" ];
};
```

---

## 4. Limitation de l'utilisation du Swap (Swappiness)

Le Swap utilise le SSD pour délester la RAM. Par défaut, Linux est configuré pour "swapper" de manière assez agressive (`vm.swappiness = 60`). Réduire cette valeur force le système à utiliser la RAM au maximum avant d'écrire sur le SSD.

```nix
{
  # Réduit l'agressivité du swap (10 est idéal pour les SSD)
  boot.kernel.sysctl."vm.swappiness" = 10;
}
```

---

## 5. Synthèse du fichier `configuration.nix` optimisé

Voici un bloc complet et propre à intégrer directement dans votre fichier de configuration principal :

```nix
{ config, pkgs, ... }: {

  # ---- OPTIMISATIONS SSD & RAM ----
  
  # Gestion de la RAM et des fichiers temporaires
  boot.tmp.useTmpfs = true;
  boot.tmp.tmpfsSize = "50%";
  nix.settings.build-dir = "/tmp";
  boot.kernel.sysctl."vm.swappiness" = 10;

  # Maintenance du SSD
  services.fstrim.enable = true;

  # Optimisation de Nix et maintenance du Store
  nix.settings.auto-optimise-store = true;
  nix.gc = {
    automatic = true;
    dates = "weekly";
    options = "--delete-older-than 14d";
  };
  
  # ---- FIN DES OPTIMISATIONS ----
}
```

---

## 6. L'architecture éphémère : Racine en RAM avec Impermanence

C'est l'optimisation absolue pour NixOS. Le concept (souvent appelé **"Erase your darlings"**) consiste à monter la racine (`/`) directement en RAM (`tmpfs`). 

* **À chaque démarrage :** Le système de fichiers racine est recréé à neuf, totalement vierge.
* **Impact SSD :** Toutes les écritures parasites (caches des navigateurs, fichiers de logs, dossiers temporaires) se font en RAM et disparaissent au reboot. Elles n'usent jamais le SSD.
* **Persistance :** Seuls les fichiers essentiels que vous aurez explicitement choisis (fichiers de configuration, clés SSH, données personnelles) sont liés physiquement au SSD via le module `impermanence`.

### Étape 1 : Configurer les disques (Exemple de structure)

Pour que cela fonctionne, vous devez séparer physiquement ce qui doit persister. Votre SSD contiendra une partition (souvent gérée avec Btrfs ou ZFS) avec un sous-volume persistant (ex: `/persistent`).

Dans votre `hardware-configuration.nix` :

```nix
{
  # 1. Montage de la racine en RAM (tmpfs)
  fileSystems."/" = {
    device = "none";
    fsType = "tmpfs";
    options = [ "defaults" "size=2G" "mode=755" ]; # Taille max allouée à la racine
  };

  # 2. Le dossier /nix (le store) reste physiquement sur le SSD
  fileSystems."/nix" = {
    device = "/dev/disk/by-uuid/votre-uuid-ssd";
    fsType = "btrfs";
    options = [ "subvol=nix" "noatime" "discard=async" ];
  };

  # 3. Le point de contrôle persistant sur le SSD
  fileSystems."/persistent" = {
    device = "/dev/disk/by-uuid/votre-uuid-ssd";
    fsType = "btrfs";
    options = [ "subvol=persistent" "noatime" ];
    neededForBoot = true; # Crucial pour que le boot trouve les clés de chiffrement/fichiers de base
  };
}
```

### Étape 2 : Activer et configurer le module Impermanence

Ajoutez le dépôt `impermanence` à vos inputs Flakes (ou via les channels), puis déclarez ce que vous souhaitez lier au SSD pour ne pas le perdre au redémarrage dans votre `configuration.nix` :

```nix
{ inputs, ... }: {
  
  imports = [ 
    inputs.impermanence.nixosModules.impermanence 
  ];

  # Déclaration des fichiers et dossiers du système à sauvegarder sur le SSD
  environment.persistence."/persistent" = {
    hideMounts = true;
    
    directories = [
      "/var/log"              # Si vous voulez garder l'historique des logs système
      "/var/lib/bluetooth"    # Vos appairages d'appareils sans fil
      "/var/lib/nixos"        # UID/GID et métadonnées système importantes
      "/var/lib/systemd/coredump"
      "/etc/NetworkManager/system-connections" # Vos réseaux Wi-Fi enregistrés
    ];
    
    files = [
      "/etc/machine-id"       # Identifiant unique de la machine (requis par systemd)
    ];

    # Persistance des données des utilisateurs (Home)
    users.votre_pseudo = {
      directories = [
        "Downloads"
        "Documents"
        "Pictures"
        "Projects"
        ".ssh"                # Vos clés SSH privées/publiques
        ".local/share/keyrings" # Le trousseau de clés de votre bureau (GNOME/KDE)
        ".config/menus"
      ];
      files = [
        ".bash_history"
      ];
    };
  };
}
```

### Avantages et Inconvénients pour le SSD

| Avantages 👍 | Inconvénients / Contraintes 👎 |
| :--- | :--- |
| **Écritures SSD réduites de ~80%** au quotidien | Nécessite une rigueur stricte (un fichier oublié est perdu au reboot) [1] |
| Confidentialité maximale (les résidus de session s'effacent) | Configuration initiale complexe (idéal avec les Flakes) |
| Le système ne s'encrasse jamais dans le temps | Consomme un peu de RAM pour stocker les fichiers racine |

### Resources

There is an alternative to the [impermanence module](https://github.com/nix-community/impermanence) called [preservation](https://github.com/nix-community/preservation/) that accomplishes roughly the same thing without a shell interpreter.

Here are some articles I referenced when putting my setup together (some already shared here):

- [The impermanence wiki entry](https://wiki.nixos.org/wiki/Impermanence)
    
- [Erase your darlings by Graham Christensen](https://grahamc.com/blog/erase-your-darlings/)
    
- [tmpfs as root by Elis Hirwing](https://elis.nu/blog/2020/05/nixos-tmpfs-as-root/)
    
- [tmpfs as home by Elis Hirwing](https://elis.nu/blog/2020/06/nixos-tmpfs-as-home/)
    
- [Impermanent NixOS: on a VM + tmpfs root + flakes + LUKS by Will Bush](https://willbush.dev/blog/impermanent-nixos/)
    
- [Encypted Btrfs Root with Opt-in State on NixOS by mt_caret](https://mt-caret.github.io/blog/posts/2020-06-29-optin-state.html)
    
- [Impermanence - In Search of Pure State by John White](https://www.hiibolt.com/p/impermanence/)
    
- [NixOS as a server, part 1: Impermanence by Guekka](https://guekka.github.io/nixos-server-1/)
    

Here are a few nicely-done configs I've looked at that use some form of opt-in state:

- [https://github.com/baitinq/nixos-config](https://github.com/baitinq/nixos-config)
    
- [https://github.com/nazarewk-iac/nix-configs](https://github.com/nazarewk-iac/nix-configs)
    
- [https://github.com/Misterio77/nix-config](https://github.com/Misterio77/nix-config)
    
- [https://gitlab.com/hmajid2301/nixicle](https://gitlab.com/hmajid2301/nixicle)
    
- [https://github.com/linyinfeng/dotfiles](https://github.com/linyinfeng/dotfiles)

