deps:
	go get github.com/go-resty/resty
	
build: 
	go build -o terraform-provider-nsx main.go

fmt:
	go fmt ./nsx 
	go fmt ../govnsx

all: deps build fmt

.PHONY: build deps fmt
