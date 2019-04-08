#!/bin/sh
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
NC='\033[0m'
DIR=$(cd "$(dirname "$0")"; pwd)

printf "${BLUE}Building service (docker image)${NC}\n"
docker build -t eclipse/che-machine-exec .

printf "${BLUE}Generating Che plug-in file...${NC}\n"
cd ${DIR}/assembly && ./build.sh
printf "${BLUE}Generated in assembly/che-service-plugin.tar.gz${NC}\n"
