# Environment variables
. "/nix/store/nl742gi1lsdwbsrwx9r6nxzf9b1334zd-hm-session-vars.sh/etc/profile.d/hm-session-vars.sh"

# Only source this once
if [[ -z "${__HM_ZSH_SESS_VARS_SOURCED-}" ]]; then
  export __HM_ZSH_SESS_VARS_SOURCED=1
  
fi
