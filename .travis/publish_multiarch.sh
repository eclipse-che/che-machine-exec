#!/bin/bash
set -e

# Create amend with images built on individual architectures
AMEND=""
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:$TAG-amd64";
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:$TAG-arm64";
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:$TAG-ppc64le";
AMEND+=" --amend ${REGISTRY}/${ORGANIZATION}/${IMAGE}:$TAG-s390x";

# Create manifest and push multiarch image
docker manifest create "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}" $AMEND
docker manifest push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}"

if [[ "$TAG" == "nightly" ]]; then
    docker manifest create "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}" $AMEND
    docker manifest push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}"
fi
