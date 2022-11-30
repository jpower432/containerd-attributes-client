GO := go

GO_BUILD_PACKAGES := ./cmd/...
GO_BUILD_BINDIR :=./bin
GIT_COMMIT := $(or $(SOURCE_GIT_COMMIT),$(shell git rev-parse --short HEAD))
GIT_TAG :="$(shell git tag | sort -V | tail -1)"


build: prep-build-dir
	$(GO) build -o $(GO_BUILD_BINDIR)/  $(GO_BUILD_PACKAGES)
.PHONY: build

prep-build-dir:
	mkdir -p ${GO_BUILD_BINDIR}
.PHONY: prep-build-dir

vendor:
	$(GO) mod tidy
	$(GO) mod verify
	$(GO) mod vendor
.PHONY: vendor

clean:
	@rm -rf ./$(GO_BUILD_BINDIR)/*
.PHONY: clean

test-unit:
	$(GO) test $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./...
.PHONY: test-unit

sanity: vendor format vet
	git diff --exit-code
.PHONY: sanity

format:
	$(GO) fmt ./...
.PHONY: format

vet:
	$(GO) vet ./...
.PHONY: vet

generate:
	protoc api/services/*/*/*.proto --go-grpc_out=. --go-grpc_opt=paths=source_relative --go_out=. --go_opt=paths=source_relative --proto_path=.
.PHONY: generate

all: clean vendor test-unit build
.PHONY: all