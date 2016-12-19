#!/bin/sh -x

# cd langserver/cmd/langserver-antha
# ( \
#   go build -race -v . && \
#   go install -race -v . && \
#   go run -race ./langserver-go.go -trace -mode ws \
# )

RACE=$1

( \
  go build -v $RACE github.com/sourcegraph/jsonrpc2 && \
  go install -v $RACE github.com/sourcegraph/jsonrpc2 && \
  go build -v $RACE github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
  go install -v $RACE github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
  ls -lah `which langserver-antha` && \
  langserver-antha -trace -mode ws \
)