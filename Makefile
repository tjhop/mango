GOCMD := go
GOFMT := ${GOCMD} fmt
GOMOD := ${GOCMD} mod
COMMIT := $(shell git rev-parse HEAD)
RELEASE_CONTAINER_NAME := mango
GOLANGCILINT_CACHE := ${CURDIR}/.golangci-lint/build/cache

# autogenerate help messages for comment lines with 2 `#`
.PHONY: help
help: ## print this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[36m%-10s\033[0m\t%s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: tidy
tidy: ## tidy modules
	${GOMOD} tidy

.PHONY: fmt
fmt: ## apply go code style formatter
	${GOFMT} -x ./...

.PHONY:	lint
lint: ## run linters
	mkdir -p ${GOLANGCILINT_CACHE} || true
	podman run --rm -v ${CURDIR}:/app -v ${GOLANGCILINT_CACHE}:/root/.cache -w /app docker.io/golangci/golangci-lint:latest golangci-lint run -v

.PHONY: build-mango
build-mango: fmt tidy lint ## build the `mango` configuration management server
	goreleaser build --clean --single-target --snapshot --output . --id "mango"

.PHONY: build-mh
build-mh: fmt tidy lint ## build `mh`, the helper tool for mango
	goreleaser build --clean --single-target --snapshot --output . --id "mh"

.PHONY: build
build: build-mango build-mh ## alias for `build-mango build-mh`

.PHONY: binary
binary: build ## alias for `build`

.PHONY: container
container: binary ## build container image with binary
	podman image build -t ${RELEASE_CONTAINER_NAME}:latest .

.PHONY: image
image: container ## alias for `container`

.PHONY: podman
podman: container ## alias for `container`

.PHONY: docker
docker: container ## alias for `container`

.PHONY: test-container
test-container: binary ## build test containers with binary for testing purposes
	podman image build -t "mango-test-ubuntu" -f Dockerfile-testbox-ubuntu .
	podman image build -t "mango-test-arch" -f Dockerfile-testbox-arch .

.PHONY: test-image
test-image: container ## alias for `container`

.PHONY: test-podman
test-podman: container ## alias for `container`

.PHONY: test-docker
test-docker: container ## alias for `container`

.PHONY: services
services: ## use podman compose to spin up local grafana, prometheus, etc
	podman-compose -f docker-compose-services.yaml up -d

.PHONY: run-test-containers
run-test-containers: test-container services ## use podman compose to spin up test containers running systemd for use with the test inventory
	podman-compose -f docker-compose-test-mango.yaml --podman-run-args="--systemd=true" up -d

.PHONY: reload-test-inventory
reload-test-inventory: run-test-inventory ## use podman to reload the mango systemd service running in the ubuntu test container
	podman-compose -f docker-compose-test-mango.yaml exec -T mango-archlinux /bin/bash -c 'systemctl reload mango.service'
	podman-compose -f docker-compose-test-mango.yaml exec -T mango-ubuntu-2204 /bin/bash -c 'systemctl reload mango.service'

.PHONY: stop
stop: ## stop test environment and any other cleanup
	podman-compose -f docker-compose-services.yaml down
	podman-compose -f docker-compose-test-mango.yaml down

.PHONY: clean
clean: stop ## alias for `stop`
