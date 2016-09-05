GO ?= go
GOPATH := $(CURDIR)/../../../..
PACKAGES := $(shell GOPATH=$(GOPATH) go list ./... | grep -v /vendor/)

all: install

build: install_deps
	GOPATH=$(GOPATH) $(GO) build

test: install_deps
	GOPATH=$(GOPATH) $(GO) test -cover $(PACKAGES)
	GOPATH=$(GOPATH) $(GO) vet $(PACKAGES)

fmt:
	GOPATH=$(GOPATH) find . -name "*.go" | xargs gofmt -w

install_deps: install_glide
	GOPATH=$(GOPATH) $(GOPATH)/bin/glide install

install_glide:
	GOPATH=$(GOPATH) $(GO) get github.com/Masterminds/glide
