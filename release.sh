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

if [ -z "${GITHUB_TAG}" ]; then
  echo "Variable GITHUB_TAG is missing"
  exit 1
fi
if [ -z "${DOCKER_IMAGE_TAG}" ]; then
  echo "Variable DOCKER_IMAGE_TAG is missing"
  exit 1
fi
if [ -z "${RELEASE_BRANCH}" ]; then
  echo "Variable RELEASE_BRANCH is missing"
  exit 1
fi


CHE_MACHINE_EXEC_IMAGE=eclipse/che-machine-exec:${DOCKER_IMAGE_TAG}
DEV_CHE_MACHINE_EXEC_IMAGE=eclipse/che-machine-exec-dev:${DOCKER_IMAGE_TAG}

# checkout to release branch
git checkout $RELEASE_BRANCH

# create and push new tag
git tag $GITHUB_TAG
git push origin $GITHUB_TAG

# checkout to new tag
git checkout $GITHUB_TAG

docker login -u ${DOCKER_HUB_LOGIN} -p ${DOCKER_HUB_PASSWORD}

# Build images.
printf "${BLUE}Building docker image ${CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker build -t ${CHE_MACHINE_EXEC_IMAGE} -f dockerfiles/ci/Dockerfile .
printf "${BLUE}Image build ${CHE_MACHINE_EXEC_IMAGE} completed.${NC}\n"

printf "${BLUE}Building docker development image ${DEV_CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker build -t ${DEV_CHE_MACHINE_EXEC_IMAGE} -f dockerfiles/dev/Dockerfile .
printf "${BLUE}Image build ${DEV_CHE_MACHINE_EXEC_IMAGE} completed.${NC}\n"

# Tag images to latest
printf "${BLUE}Tag docker image ${CHE_MACHINE_EXEC_IMAGE} to latest\n"
docker tag  ${CHE_MACHINE_EXEC_IMAGE} eclipse/che-machine-exec:latest

printf "${BLUE}Tag docker development image ${DEV_CHE_MACHINE_EXEC_IMAGE} to latest\n"
docker tag ${DEV_CHE_MACHINE_EXEC_IMAGE} eclipse/che-machine-exec-dev:latest

# Push images.
printf "${BLUE}Push docker image ${CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker push ${CHE_MACHINE_EXEC_IMAGE}
printf "${BLUE}Image ${CHE_MACHINE_EXEC_IMAGE} pushed.${NC}\n"

printf "${BLUE}Push docker image eclipse/che-machine-exec:latest ==>${NC}\n"
docker push eclipse/che-machine-exec:latest
printf "${BLUE}Image eclipse/che-machine-exec:latest pushed.${NC}\n"

printf "${BLUE}Push docker image ${DEV_CHE_MACHINE_EXEC_IMAGE} ==>${NC}\n"
docker push ${DEV_CHE_MACHINE_EXEC_IMAGE}
printf "${BLUE}Image ${DEV_CHE_MACHINE_EXEC_IMAGE} pushed.${NC}\n"

printf "${BLUE}Push docker image eclipse/che-machine-exec-dev:latest ==>${NC}\n"
docker push eclipse/che-machine-exec-dev:latest
printf "${BLUE}Image eclipse/che-machine-exec-dev:latest pushed.${NC}\n"


printf "${GREEN}Done. All images successfully pushed.${NC}\n"
