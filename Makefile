SHELL := /bin/bash
VERSION ?= 0.1.0
OUT ?= dist

.PHONY: all build test vet fmtcheck clean package repo

all: build

build:
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w" \
		-o $(OUT)/cpz-app ./cmd/cpz-app

test:
	go test ./...

vet:
	go vet ./...

fmtcheck:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

# Requires an Arch environment with makepkg and repo-add.
package: test fmtcheck
	./tools/build-repo.sh

repo: package

clean:
	rm -rf $(OUT) packaging/cpz-system-apps/pkg packaging/cpz-system-apps/src \
		packaging/cpz-apps-catalog/pkg packaging/cpz-apps-catalog/src \
		packaging/cpz-apps-catalog/apps.json
