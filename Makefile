GO ?= go
VERSION ?= dev
BIN_DIR ?= $(CURDIR)/bin
LOCAL_BIN ?= $(HOME)/.local/bin
LDFLAGS := -s -w -X github.com/jo-cube/pbl/internal/buildinfo.version=$(VERSION)

.PHONY: build test run install clean

build:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -ldflags '$(LDFLAGS)' -o "$(BIN_DIR)/pbl" ./cmd/pbl

test:
	$(GO) test ./...

run:
	$(GO) run -ldflags '$(LDFLAGS)' ./cmd/pbl $(ARGS)

install:
	@mkdir -p "$(LOCAL_BIN)"
	GOBIN="$(LOCAL_BIN)" $(GO) install -ldflags '$(LDFLAGS)' ./cmd/pbl

clean:
	rm -rf "$(BIN_DIR)"
