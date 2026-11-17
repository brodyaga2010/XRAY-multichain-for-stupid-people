# Сборка панели: один статический бинарник для Linux (amd64/arm64) с вшитым фронтендом.
#
#   make web     собрать фронтенд и скопировать в internal/web/dist
#   make build   собрать бинарники в dist/
#   make test    go vet + go test
#   make all     web + test + build

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
SITE_NAME ?=
LDFLAGS   := -s -w -X main.version=$(VERSION)
WEB_DIST  := internal/web/dist

.PHONY: all web web-reset build build-amd64 build-arm64 test fmt clean

all: web test build

web:
	cd ../frontend && npm ci && VITE_API_BASE=/api VITE_SITE_NAME="$(SITE_NAME)" npm run build
	find $(WEB_DIST) -mindepth 1 ! -name .gitkeep -delete
	cp -R ../frontend/dist/. $(WEB_DIST)/

# Вернуть заглушку index.html (после make web она заменяется настоящей сборкой).
web-reset:
	find $(WEB_DIST) -mindepth 1 ! -name .gitkeep -delete
	git checkout -- $(WEB_DIST)/index.html

build: build-amd64 build-arm64

build-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/xray-panel-linux-amd64 ./cmd/xray-panel

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/xray-panel-linux-arm64 ./cmd/xray-panel

test:
	go vet ./...
	go test ./...

fmt:
	gofmt -w .

clean:
	rm -rf dist
