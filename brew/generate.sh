#!/bin/bash

die () {
    echo >&2 "$@"
    exit 1
}

if [ "$1" = "" ]
then
    die "Usage: $0 <major.minor.build>"
fi

REPO_OWNER=sourcegraph
REPO_NAME=go-langserver
WORK_DIR=`mktemp -d 2>/dev/null || mktemp -d -t 'mytmpdir'`
GOPATH=$WORK_DIR
VERSION=$1
TEMPLATE=`cat $PWD/go-langserver.rb.template`
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
OUT_FILE=$DIR/go-langserver.rb

echo "Calculating SHA256 sum"
SHA256SUM=`curl -sL https://github.com/$REPO_OWNER/$REPO_NAME/archive/v$VERSION.tar.gz | sha256sum | awk '{print $1}' || die "Failed to calculate SHA256 checksum"`
echo "Downloading source code"
mkdir -p $WORK_DIR/src/github.com/$REPO_OWNER/$REPO_NAME
git clone -q https://github.com/$REPO_OWNER/$REPO_NAME $WORK_DIR/src/github.com/$REPO_OWNER/$REPO_NAME || (rm -rf $WORK_DIR && die "Failed to clone repository")
cd $WORK_DIR/src/github.com/$REPO_OWNER/$REPO_NAME
git checkout -q v$VERSION || (rm -rf $WORK_DIR && die "Failed to switch repository to specific version")
echo "Fetching dependencies"
go get -d  ./... || (rm -rf $WORK_DIR && die "Failed to fetch Go dependencies")
echo "Installing homebrew-go-resources"
go get github.com/samertm/homebrew-go-resources || (rm -rf $WORK_DIR && die "Failed to fetch homebrew-go-resources tool")
echo "Processing dependencies"
GO_RESOURCES=`$GOPATH/bin/homebrew-go-resources github.com/$REPO_OWNER/$REPO_NAME/... || (rm -rf $WORK_DIR && die "Failed to generate list of Go dependencies")`
# Removing leading and trailing spaces to make `brew audit --strict` happy
GO_RESOURCES="${GO_RESOURCES#"${GO_RESOURCES%%[![:space:]]*}"}"
GO_RESOURCES="${GO_RESOURCES#"${GO_RESOURCES##*[![:space:]]}"}"
TEMPLATE=${TEMPLATE//#\{GO_RESOURCES\}/${GO_RESOURCES}}
TEMPLATE=${TEMPLATE//#\{VERSION\}/${VERSION}}
TEMPLATE=${TEMPLATE//#\{REPO_OWNER\}/${REPO_OWNER}}
TEMPLATE=${TEMPLATE//#\{REPO_NAME\}/${REPO_NAME}}
TEMPLATE=${TEMPLATE//#\{SHA256SUM\}/${SHA256SUM}}
echo "$TEMPLATE" > $OUT_FILE
rm -rf $WORK_DIR

