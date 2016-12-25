#!/bin/sh -xv

# go tool compile -help
BUILD_FLAGS=(`echo -v -gcflags '-K -N -l -f -g -h -i -l -v -w'`)

( \
  go build $BUILD_FLAGS github.com/sourcegraph/jsonrpc2 && \
  go install $BUILD_FLAGS github.com/sourcegraph/jsonrpc2 && \
  go build $BUILD_FLAGS github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
  go install $BUILD_FLAGS github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
  ls -lah `which langserver-antha` && \
  langserver-antha -trace -mode ws \
)