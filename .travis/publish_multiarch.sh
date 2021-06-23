#!/bin/bash
set -e

# Create amend with images built on individual architectures
AMEND=""
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-amd64";
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-arm64";
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-ppc64le";
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-s390x";

# Create manifest and push multiarch image
docker manifest create "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}" $AMEND
docker manifest push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}"

if [[ "$TAG" == "next" ]]; then
    docker manifest create "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}-${TRAVIS_TAG}" $AMEND
    docker manifest push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}-${TRAVIS_TAG}"
fi
