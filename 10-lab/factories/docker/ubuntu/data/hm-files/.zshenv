if [[ ! -o login ]]; then
  # Environment variables
  . "/nix/store/nl742gi1lsdwbsrwx9r6nxzf9b1334zd-hm-session-vars.sh/etc/profile.d/hm-session-vars.sh"

  # Only source this once
  if [[ -z "${__HM_ZSH_SESS_VARS_SOURCED-}" ]]; then
    export __HM_ZSH_SESS_VARS_SOURCED=1
    
  fi
fi

ZSH=/nix/store/fdcqkb63hyh4h4rdkshrrnfwa94k17sm-oh-my-zsh-2026-08-16/share/oh-my-zsh;
ZSH_CACHE_DIR=/home/dev/.cache/oh-my-zsh;
