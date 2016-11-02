.PHONY: docker-image

TAG ?= latest

docker-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o langserver-go .
	docker build -t us.gcr.io/sourcegraph-dev/langserver-go:$(TAG) .
