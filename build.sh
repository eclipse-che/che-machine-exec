#!/bin/sh
set -e
set -u

BLUE='\033[1;34m'
NC='\033[0m'
DIR=$(cd "$(dirname "$0")"; pwd)

printf "${BLUE}Building service (docker image)${NC}\n"

docker build --no-cache -t eclipse/che-machine-exec:latest .
GIT_HASH=$(git rev-parse --short=7 HEAD)
docker tag eclipse/che-machine-exec:latest eclipse/che-machine-exec:${GIT_HASH}

printf "${BLUE}Generating Che plug-in file...${NC}\n"
cd ${DIR}/assembly && ./build.sh
printf "${BLUE}Generated in assembly/che-service-plugin.tar.gz${NC}\n"
