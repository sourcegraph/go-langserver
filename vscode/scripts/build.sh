#!/bin/sh

# we need to link in the our custom version of the
# vscode-languageclient, so use yarn to link it.

echo "---"
DEPS="vscode-languageclient"
echo "BUILDING `pwd`"
echo "DEPS: $DEPS"

yarn link "${DEPS}"
yarn run compile
echo "---"