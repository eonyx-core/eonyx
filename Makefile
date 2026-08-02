MODULE  := github.com/eonyx-core/eonyx
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/version.Version=$(VERSION)

ifeq ($(OS),Windows_NT)
EXE := .exe
endif

.PHONY: build install test vet version

build:
	go build -ldflags "$(LDFLAGS)" -o bin/eonyx$(EXE) .

install:
	go install -ldflags "$(LDFLAGS)" .

test:
	go test ./...

vet:
	go vet ./...

version:
	@echo $(VERSION)
