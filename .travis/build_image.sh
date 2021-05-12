#!/bin/bash
#
# Copyright (c) 2021 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#
# Contributors:
#   Red Hat, Inc. - initial API and implementation
set -e

# Build images
docker build -f build/dockerfiles/Dockerfile -t "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}" .
docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}"

# Tag image with short_sha in case of next build
if [[ "$TAG" == "next" ]]; then
    docker tag "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${TAG}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}" "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}"
    docker push "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${SHORT_SHA}-${TRAVIS_TAG}-${TRAVIS_CPU_ARCH}"
fi
