GOCMD := go
GOFMT := ${GOCMD} fmt
GOMOD := ${GOCMD} mod
COMMIT := $(shell git rev-parse HEAD)
RELEASE_CONTAINER_NAME := "mango"
GOLANGCILINT_CACHE := ${CURDIR}/.golangci-lint/build/cache

## help:			print this help message
.PHONY: help
help: Makefile
	# autogenerate help messages for comment lines with 2 `#`
	@sed -n 's/^##//p' $<

## tidy:			tidy modules
tidy:
	${GOMOD} tidy

## fmt:			apply go code style formatter
fmt:
	${GOFMT} -x ./...

## lint:			run linters
lint:
	mkdir -p ${GOLANGCILINT_CACHE} || true
	podman run --rm -v ${CURDIR}:/app -v ${GOLANGCILINT_CACHE}:/root/.cache -w /app docker.io/golangci/golangci-lint:latest golangci-lint run -v

## binary:		build a binary
binary: fmt tidy lint
	goreleaser build --clean --single-target --snapshot --output .

## build:			alias for `binary`
build: binary

## container: 		build container image with binary
container: binary
	podman image build -t "${RELEASE_CONTAINER_NAME}:latest" .

## image:			alias for `container`
image: container

## podman:		alias for `container`
podman: container

## docker:		alias for `container`
docker: container

## test-container:	build test containers with binary for testing purposes
test-container: binary container
	podman image build -t "mango-test-ubuntu" -f Dockerfile-testbox-ubuntu .
	podman image build -t "mango-test-arch" -f Dockerfile-testbox-arch .

## test-image:		alias for `container`
test-image: container

## test-podman:		alias for `container`
test-podman: container

## test-docker:		alias for `container`
test-docker: container

## services:		use podman compose to spin up local grafana, prometheus, etc
services:
	podman-compose -f docker-compose-services.yaml up -d

## run-test-containers	use podman compose to spin up test containers running systemd for use with the test inventory
run-test-containers: test-container services
	podman-compose -f docker-compose-test-mango.yaml --podman-run-args="--systemd=true" up -d

## reload-test-inventory: use podman to reload the mango systemd service running in the ubuntu test container
reload-test-inventory: run-test-inventory
	podman-compose -f docker-compose-test-mango.yaml exec -T mango-archlinux /bin/bash -c 'systemctl reload mango.service'
	podman-compose -f docker-compose-test-mango.yaml exec -T mango-ubuntu-2204 /bin/bash -c 'systemctl reload mango.service'

## stop:			stop test environment and any other cleanup
stop:
	podman-compose -f docker-compose-services.yaml down
	podman-compose -f docker-compose-test-mango.yaml down

## clean: 		alias for `stop`
