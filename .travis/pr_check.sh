#!/bin/bash

set -e

# Build che-machine-exec binary and execute unit tests
CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -ldflags '-w -s' -a -installsuffix cgo -o che-machine-exec .
export CHE_WORKSPACE_ID=test_id
go test ./... -test.v

# Build image with pr-check tag 
docker build -f build/dockerfiles/Dockerfile -t "${REGISTRY}/${ORGANIZATION}/${IMAGE}:pr-check-${TRAVIS_CPU_ARCH}" . 
