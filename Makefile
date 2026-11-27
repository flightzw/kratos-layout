GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always 2>/dev/null || echo dev)
IMAGE ?= kratos-layout
GO_VERSION ?= $(shell awk '$$1 == "go" { print $$2; exit }' go.mod)
GOPROXY ?= https://proxy.golang.org,direct
APP_PATH ?= ./cmd/...

.PHONY: init
# init env
init:
	@test -e .env || test -L .env || cp .env.example .env
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install github.com/mikefarah/yq/v4@latest
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
	go install github.com/flightzw/gorm-genkit/cmd/genkit@latest

.PHONY: config
# generate internal proto
config:
	buf generate --template manifest/buf/buf.gen.config.yaml

.PHONY: errno
# generate errno proto
errno:
	buf generate --template manifest/buf/buf.gen.errno.yaml

.PHONY: api
# generate api proto
api:
	buf generate --template manifest/buf/buf.gen.yaml
	yq -o=json docs/openapi/openapi.yaml > docs/openapi/openapi.json && rm docs/openapi/openapi.yaml

.PHONY: gormgen
# generate gorm dao
gormgen:
	genkit -c gorm.gen.yaml

.PHONY: wire
# generate wire injectors
wire:
	wire ./cmd/...

.PHONY: build
# build
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./...

.PHONY: docker
# build docker image
docker:
	docker build -f manifest/docker/Dockerfile -t "$(IMAGE)" \
		--build-arg GO_VERSION="$(GO_VERSION)" --build-arg GOPROXY="$(GOPROXY)" \
		--build-arg APP_PATH="$(APP_PATH)" --build-arg VERSION="$(VERSION)" .

.PHONY: generate
# generate
generate:
	go generate ./...
	go mod tidy

.PHONY: all
# generate all
all:
	make api
	make config
	make generate

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
