#!/bin/sh
set -e
set -u

BLUE='\033[1;34m'
NC='\033[0m'
DIR=$(cd "$(dirname "$0")"; pwd)

printf "${BLUE}Building service (docker image)${NC}\n"
docker build -t aandrienko/che-machine-exec .

printf "${BLUE}Generating Che plug-in file...${NC}\n"
cd ${DIR}/assembly && ./build.sh
printf "${BLUE}Generated in assembly/che-service-plugin.tar.gz${NC}\n"
