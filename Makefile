SHELL := /bin/bash
INSTALL_DIR ?= /usr/bin
PLUGIN_BIN ?= github-stats
PLUGIN_DEPENDENCIES := $(shell find . -name "*.go")
# if you want to execute gotest with verbosity, set this flag to `true`.
TEST_VERBOSE ?= true

.PHONY: build init format test install
build: format test $(PLUGIN_BIN)

init:
	aqua install

format:
	go fmt ./...

test:
ifeq ($(TEST_VERBOSE), true)
	go test -v ./... -race -count=1
else
	go test ./... -race -count=1
endif

$(PLUGIN_BIN): $(PLUGIN_DEPENDENCIES)
	go build -o $(PLUGIN_BIN) ./main.go

install: $(PLUGIN_BIN)
