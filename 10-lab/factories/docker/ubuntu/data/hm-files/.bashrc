

# Commands that should be applied only for interactive shells.
[[ $- == *i* ]] || return

HISTFILESIZE=100000
HISTSIZE=10000

shopt -s histappend
shopt -s extglob
shopt -s globstar
shopt -s checkjobs

alias -- ll='ls -lah'

if [[ ! -v BASH_COMPLETION_VERSINFO ]]; then
  . "/nix/store/d4577r3iadywsg81g3vcd5b8iqw8r089-bash-completion-2.18.0/etc/profile.d/bash_completion.sh"
fi

eval "$(/nix/store/9q13w144w01b3mmw7x9aylsjir6jy98a-direnv-2.37.1/bin/direnv hook bash)"

