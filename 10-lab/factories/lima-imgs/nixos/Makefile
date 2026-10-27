build-image:
	@nix build .#image --option system-features "benchmark big-parallel nixos-test uid-range kvm"
	@cp result/nixos.img .
	@sudo rm -fr result

build:
	@sudo nixos-rebuild switch --flake .#lima || true
	@nixos-rebuild list-generations
