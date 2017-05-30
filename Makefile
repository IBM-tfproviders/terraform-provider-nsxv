PKG_LIST=$(shell go list ./...)
BUILD_VERSION=EoSP9


deps:
	go get github.com/go-resty/resty
	
build: version.go
	go build -o terraform-provider-nsxv  github.com/IBM-tfproviders/terraform-provider-nsxv

version.go:
	@echo -e "package main\nconst BuildVersion = \"$(BUILD_VERSION)\"" > version.go
	go fmt version.go

fmt:
	go fmt $(PKG_LIST)

clean:
	rm -f version.go terraform-provider-nsxv

all: deps build fmt

.PHONY: build deps fmt
