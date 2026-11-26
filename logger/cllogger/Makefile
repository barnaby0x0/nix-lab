# Configuration
IMAGE_NAME ?= logger-service
TAG         ?= latest
REGISTRY    ?=
PLATFORMS  ?= linux/amd64,linux/arm64

.PHONY: build build-amd64 build-arm64 build-multi push run clean

build:
	docker buildx build \
		--platform $$(docker version --format '{{.Server.Arch}}' | sed 's/x86_64/amd64/;s/aarch64/arm64/') \
		-t $(IMAGE_NAME):$(TAG) \
		--load \
		.

build-amd64:
	docker buildx build \
		--platform linux/amd64 \
		-t $(IMAGE_NAME):$(TAG) \
		--load \
		.

build-arm64:
	docker buildx build \
		--platform linux/arm64 \
		-t $(IMAGE_NAME):$(TAG) \
		--load \
		.

build-multi:
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(IMAGE_NAME):$(TAG) \
		.

push:
	docker buildx build \
		--platform $(PLATFORMS) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(TAG) \
		--push \
		.

run:
	docker run --rm -p 5000:5000 $(IMAGE_NAME):$(TAG)

clean:
	docker image rm $(IMAGE_NAME):$(TAG)
