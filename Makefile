SHELL := /usr/bin/env bash
.PHONY: test build package clean session-key

PLUGIN_NAME := navidrome-lastfm-exclusion
WASM_FILE := plugin.wasm
TINYGO := $(shell command -v tinygo 2> /dev/null)

test:
	go test -race ./...

build:
ifdef TINYGO
	tinygo build -opt=2 -scheduler=none -no-debug -o $(WASM_FILE) -target wasip1 -buildmode=c-shared .
else
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o $(WASM_FILE) .
endif

package: build
	zip $(PLUGIN_NAME).ndp $(WASM_FILE) manifest.json

clean:
	rm -f $(WASM_FILE) $(PLUGIN_NAME).ndp

# Interactive one-time helper to mint a Last.fm session key for a user.
# Usage: make session-key APIKEY=xxx APISECRET=yyy
session-key:
	cd cmd/session-key && go run . -apikey "$(APIKEY)" -apisecret "$(APISECRET)"
