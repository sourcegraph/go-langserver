#!/bin/sh -x

cd langserver/cmd/langserver-antha
(go build -race -v . && \
	go install -race -v . && \
	go run -race ./langserver-go.go -trace -mode ws)