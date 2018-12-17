#!/bin/bash
set -ex
cd $(dirname "${BASH_SOURCE[0]}")

# Build image
VERSION=$(printf "%05d" $BUILDKITE_BUILD_NUMBER)_$(date +%Y-%m-%d)_$(git rev-parse --short HEAD)
docker build -t sourcegraph/lang-go:$VERSION .

# Upload to Docker Hub
docker push sourcegraph/lang-go:$VERSION
docker tag sourcegraph/lang-go:$VERSION sourcegraph/lang-go:latest
docker push sourcegraph/lang-go:latest
docker tag sourcegraph/lang-go:$VERSION sourcegraph/lang-go:insiders
docker push sourcegraph/lang-go:insiders
