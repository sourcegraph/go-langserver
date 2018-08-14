#!/bin/sh

set -o errexit
set -o nounset

TARGETS=$(for d in "$@"; do echo ./$d/...; done)

echo -n "Checking gofmt: "
ERRS=$(find "$@" -type f -name \*.go | xargs gofmt -l 2>&1 || true)
if [ -n "${ERRS}" ]; then
    echo "FAIL - the following files need to be gofmt'ed:"
    for e in ${ERRS}; do
        echo "    $e"
    done
    echo
    exit 1
fi
echo "PASS"
echo

echo "Running tests:"
echo ${TARGETS}
go test -i ${TARGETS}
go test -timeout 5m  -race ${TARGETS}
echo