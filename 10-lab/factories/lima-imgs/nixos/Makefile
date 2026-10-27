build-image:
	@nix build .#image --option system-features "benchmark big-parallel nixos-test uid-range kvm"

build:
	@sudo nixos-rebuild switch --flake .#lima || true
	@nixos-rebuild list-generations
