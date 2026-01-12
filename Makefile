# Overridable vars
# Sets the image repository
PGEDGE_IMAGE_REPO ?= 127.0.0.1:5000/pgedge-postgres
# When set to "1", images will be rebuilt and republished
PGEDGE_IMAGE_REPUBLISH ?= 0
# When set to "1", build and publish steps will be skipped
PGEDGE_IMAGE_DRY_RUN ?= 0
# When set to "1", build will run with the --no-cache flag
PGEDGE_IMAGE_NO_CACHE ?= 0
# When set to a postgres major.minor version, will restrict build and publish to
# that postgres version.
PGEDGE_IMAGE_ONLY_POSTGRES_VERSION ?=
# When set to a spock major.minor.patch version, will restrict build and publish
# to that spock version.
PGEDGE_IMAGE_ONLY_SPOCK_VERSION ?=
# When set to a specific architecture, e.g. "amd64" or "arm64", will restrict
# build and publish to that architecture.
# WARNING: this should only be used for testing because the resulting manifest
# will only be usable on one architecture.
PGEDGE_IMAGE_ONLY_ARCH ?=
# When set to "1", images will be signed with cosign after being published
PGEDGE_SIGN_IMAGES ?= 1
# These builders are defined in the main Makefile. In CI, we run builds
# sequentially.
BUILDX_BUILDER=$(if $(CI),"pgedge-images-ci","pgedge-images")
BUILDX_CONFIG=$(if $(CI),"./buildkit.ci.toml","./buildkit.toml")

.PHONY: start-local-registry
start-local-registry:
	docker service create --name registry --publish published=5000,target=5000 registry:2
	
.PHONY: buildx-init
buildx-init:
	docker buildx create \
		--name=$(BUILDX_BUILDER) \
		--platform=linux/arm64,linux/amd64 \
		--config=$(BUILDX_CONFIG)

.PHONY: pgedge-images
pgedge-images:
	PGEDGE_IMAGE_REPO=$(PGEDGE_IMAGE_REPO) \
	PGEDGE_IMAGE_REPUBLISH=$(PGEDGE_IMAGE_REPUBLISH) \
	PGEDGE_IMAGE_DRY_RUN=$(PGEDGE_IMAGE_DRY_RUN) \
	PGEDGE_IMAGE_NO_CACHE=$(PGEDGE_IMAGE_NO_CACHE) \
	PGEDGE_IMAGE_ONLY_POSTGRES_VERSION=$(PGEDGE_IMAGE_ONLY_POSTGRES_VERSION) \
	PGEDGE_IMAGE_ONLY_SPOCK_VERSION=$(PGEDGE_IMAGE_ONLY_SPOCK_VERSION) \
	PGEDGE_IMAGE_ONLY_ARCH=$(PGEDGE_IMAGE_ONLY_ARCH) \
	BUILDX_BUILDER=$(BUILDX_BUILDER) \
	./scripts/build_pgedge_images.py

# Test targets
# IMAGE: The full image reference to test (e.g., ghcr.io/pgedge/pgedge-postgres:17-spock5-standard)
# FLAVOR: The image flavor, either "minimal" or "standard" (required)
#
# Examples:
#   make test-image IMAGE=ghcr.io/pgedge/pgedge-postgres:17-spock5-standard FLAVOR=standard
#   make test-image IMAGE=127.0.0.1:5000/pgedge-postgres:17.7-spock5.0.4-minimal-3 FLAVOR=minimal
IMAGE ?=
FLAVOR ?=

.PHONY: test-image
test-image:
ifndef IMAGE
	$(error IMAGE is required. Usage: make test-image IMAGE=<image> FLAVOR=<minimal|standard>)
endif
ifndef FLAVOR
	$(error FLAVOR is required. Usage: make test-image IMAGE=<image> FLAVOR=<minimal|standard>)
endif
	cd tests && go run main.go -image $(IMAGE) -flavor $(FLAVOR)

.PHONY: latest-tags
latest-tags:
	@PGEDGE_LIST_LATEST_TAGS=1 ./scripts/build_pgedge_images.py
