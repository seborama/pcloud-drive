SHELL=/bin/bash

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	OS_NAME = linux
endif
ifeq ($(UNAME_S),Darwin)
	OS_NAME = osx
endif
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),x86_64)
	GO_RACE = -race
endif

.phony: build
build:
	@go build -v -o ./bin/pcloud-drive ./cmd && \
	 echo "Binary created at ./bin/pcloud-drive"

.phony: assets
assets:
	@echo 'GOOS="linux" GOARCH="arm64"' && \
	 GOOS="linux" GOARCH="arm64" go build -o ./bin/pcloud-drive ./cmd/ && \
	 cd ./bin && \
	 zip -m -9 -o pcloud-drive-linux-arm64.zip pcloud-drive

	@echo 'GOOS="linux" GOARCH="amd64"' && \
	 GOOS="linux" GOARCH="amd64" go build -o ./bin/pcloud-drive ./cmd/ && \
	 cd ./bin && \
	 zip -m -9 -o pcloud-drive-linux-amd64.zip pcloud-drive

.phony: test
test:
	@[ -n "$$GO_PCLOUD_USERNAME" ] || read -p "user? " GO_PCLOUD_USERNAME ; \
	 [ -n "$$GO_PCLOUD_PASSWORD" ] || { read -s -p "pass? " GO_PCLOUD_PASSWORD && echo; } ; \
	 [ -n "$$GO_PCLOUD_TFA_CODE" ] || { read -s -p "tfa code? " GO_PCLOUD_TFA_CODE && echo; } ; \
	 GO_PCLOUD_USERNAME="$$GO_PCLOUD_USERNAME" GO_PCLOUD_PASSWORD="$$GO_PCLOUD_PASSWORD" GO_PCLOUD_TFA_CODE="$$GO_PCLOUD_TFA_CODE" go test -v -count 1 $(GO_RACE) -timeout 20s ./sdk/...

.phony: lint
lint:
	@which golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.33.0
	@GOOS="linux" golangci-lint run ./...


