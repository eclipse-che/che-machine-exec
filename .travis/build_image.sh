#!/bin/bash
set -e

# Build images
docker build -f build/dockerfiles/Dockerfile -t "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}" .
docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}"

# Tag image with short_sha in case of next build
if [[ "$TAG" == "next" ]]; then
    docker tag "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}" "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}"
    docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}"
fi
