GOCMD := go
GOFMT := ${GOCMD} fmt
GOMOD := ${GOCMD} mod
COMMIT := $(shell git rev-parse HEAD)
TEST_CONTAINER_NAME := "mango-test-ubuntu"
RELEASE_CONTAINER_NAME := "mango"

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

## lint:		run linters
lint:
	golangci-lint run

## binary:		build a binary
binary: fmt
	goreleaser build --clean --single-target --snapshot --output .

## build:			alias for `binary`
build: binary

## docker:		build docker container with binary
docker: binary
	podman image build -t "${RELEASE_CONTAINER_NAME}:latest" .

## test-docker:		build ubuntu docker container with binary for testing purposes
test-docker: binary docker
	podman image build -t "${TEST_CONTAINER_NAME}:latest" -f Dockerfile-testing .

## services:		use docker compose to spin up local grafana, prometheus, etc
services:
	docker compose up -d

## run-test-inventory:	use podman to create an ubuntu-systemd container that runs mango with the test inventory
run-test-inventory: test-docker services
	# TODO: put containers onto their own network? using host networking is convenience/laziness, at the moment.
	podman container start ${TEST_CONTAINER_NAME} 2>/dev/null || \
		podman container run -d \
			--net=host \
			--systemd=true \
			--hostname testbox \
			-v ./test/mockup/inventory:/opt/mango/inventory/:ro \
			--name "${TEST_CONTAINER_NAME}" \
			"${TEST_CONTAINER_NAME}:latest"

## reload-test-inventory: use podman to reload the mango systemd service running in the ubuntu test container
reload-test-inventory: run-test-inventory
	podman container exec -it "${TEST_CONTAINER_NAME}" /bin/bash -c 'systemctl reload mango.service'

## clean:			stop test environment and any other cleanup
clean:
	docker compose down
	podman container stop "${TEST_CONTAINER_NAME}" 2>/dev/null || true
	podman container stop "${RELEASE_CONTAINER_NAME}" 2>/dev/null || true
	podman container rm "${TEST_CONTAINER_NAME}" 2>/dev/null || true
	podman container rm "${RELEASE_CONTAINER_NAME}" 2>/dev/null || true
