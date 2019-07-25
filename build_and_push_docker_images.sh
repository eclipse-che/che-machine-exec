#!/bin/bash
#
# Copyright (c) 2012-2019 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#

set -e
set -u

BLUE='\033[1;34m'
GREEN='\033[32m'
NC='\033[0m'

DOCKER_IMAGE_TAG="0.0.1"

CHE_MACHINE_EXEC_IMAGE=eclipse/che-machine-exec:${DOCKER_IMAGE_TAG}
DEV_CHE_MACHINE_EXEC_IMAGE=eclipse/che-machine-exec-dev:${DOCKER_IMAGE_TAG}

# Build images.
printf "${BLUE}Building docker image ${CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker build -t ${CHE_MACHINE_EXEC_IMAGE} -f dockerfiles/ci/Dockerfile .
printf "${BLUE}Image build ${CHE_MACHINE_EXEC_IMAGE} completed.${NC}\n"

printf "${BLUE}Building development image ${DEV_CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker build -t ${DEV_CHE_MACHINE_EXEC_IMAGE} -f dockerfiles/dev/Dockerfile .
printf "${BLUE}Image build ${DEV_CHE_MACHINE_EXEC_IMAGE} completed.${NC}\n"

# Push images.
printf "${BLUE}Push docker image ${CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker push ${CHE_MACHINE_EXEC_IMAGE}
printf "${BLUE}Image ${CHE_MACHINE_EXEC_IMAGE} pushed.${NC}\n"

printf "${BLUE}Push docker image ${DEV_CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker push ${DEV_CHE_MACHINE_EXEC_IMAGE}
printf "${BLUE}Image ${DEV_CHE_MACHINE_EXEC_IMAGE} pushed.${NC}\n"

printf "${GREEN}Done. All images successfully pushed.${NC}\n"
