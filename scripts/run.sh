#!/bin/sh -x

# cd langserver/cmd/langserver-antha
# ( \
#   go build -race -v . && \
#   go install -race -v . && \
#   go run -race ./langserver-go.go -trace -mode ws \
# )

( \
  go install -x -v -a -race github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
  ls -lah `which langserver-antha` && \
  langserver-go.go -trace -mode ws \
)