SHELL := /bin/bash

GOPATH := $(PWD)
export GOPATH

.PHONY: map

all: build

dep:
	mkdir -p src
	cd src && dep init

build:
	go build -o bin/map

get:
	go get -v github.com/spf13/cobra

test:
	go test .

clean:
	rm -rf bin/ pkg/

linux:
	env GOOS=linux GOARCH=amd64 make

releases:
	for arch in 386 amd64 ; do for os in darwin linux ; do \
		make GOARCH=$$arch GOOS=$$os build-release ; \
	done ; done

build-release:
	go build -o bin/map_$(GOOS)_$(GOARCH)
