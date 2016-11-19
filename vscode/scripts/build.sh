#!/bin/sh

# we need to link in the our custom version of the
# vscode-languageclient, so use yarn to link it.

echo "---"
DEPS="vscode-jsonrpc vscode-languageserver-types vscode-languageclient"
echo "BUILDING `pwd`"
echo "DEPS: $DEPS"

for DEP in ${DEPS}; do
    yarn link "${DEP}"
    yarn run compile
done

echo "---"