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

# Release process automation script.
# Used to create branch/tag, update VERSION files
# and and trigger release by force pushing changes to the release branch

# set to 1 to actually trigger changes in the release branch
TRIGGER_RELEASE=0
NOCOMMIT=0

REGISTRY="quay.io"
DOCKERFILE="build/dockerfiles/Dockerfile"
ORGANIZATION="eclipse"
IMAGE="che-machine-exec"

while [[ "$#" -gt 0 ]]; do
  case $1 in
    '-t'|'--trigger-release') TRIGGER_RELEASE=1; shift 0;;
    '-v'|'--version') VERSION="$2"; shift 1;;
    '-n'|'--no-commit') NOCOMMIT=1; shift 0;;
  esac
  shift 1
done

usage ()
{
  echo "Usage: $0 --version [VERSION TO RELEASE] [--trigger-release]"
  echo "Example: $0 --version 7.7.0 --trigger-release"; echo
}

if [[ ! ${VERSION} ]]; then
  usage
  exit 1
fi

releaseMachineExec() {
  # docker buildx includes automated push to registry, so build using tag we want published, not just local ${IMAGE}
  docker buildx build \
    --tag "${REGISTRY}/${ORGANIZATION}/${IMAGE}:${VERSION}" --push \
    -f ./${DOCKERFILE} . --platform "linux/amd64,linux/ppc64le,linux/arm64" | cat
  echo "Pushed ${REGISTRY}/${ORGANIZATION}/${IMAGE}:${VERSION}"
}

# derive branch from version
BRANCH=${VERSION%.*}.x

# if doing a .0 release, use main; if doing a .z release, use $BRANCH
if [[ ${VERSION} == *".0" ]]; then
  BASEBRANCH="main"
else
  BASEBRANCH="${BRANCH}"
fi

# create new branch off ${BASEBRANCH} (or check out latest commits if branch already exists), then push to origin
if [[ "${BASEBRANCH}" != "${BRANCH}" ]]; then
  git branch "${BRANCH}" || git checkout "${BRANCH}" && git pull origin "${BRANCH}"
  git push origin "${BRANCH}"
  git fetch origin "${BRANCH}:${BRANCH}" || true
  git checkout "${BRANCH}"
else
  git fetch origin "${BRANCH}:${BRANCH}" || true
  git checkout ${BRANCH}
fi
set -e

# change VERSION file
echo "${VERSION}" > VERSION

# commit change into branch
if [[ ${NOCOMMIT} -eq 0 ]]; then
  COMMIT_MSG="[release] Bump to ${VERSION} in ${BRANCH}"
  git commit -s -m "${COMMIT_MSG}" VERSION
  git pull origin "${BRANCH}"
  git push origin "${BRANCH}"
fi

if [[ $TRIGGER_RELEASE -eq 1 ]]; then
  # push new branch to release branch to trigger CI build
  releaseMachineExec

  # tag the release
  git checkout "${BRANCH}"
  git tag "${VERSION}"
  git push origin "${VERSION}"
fi

# now update ${BASEBRANCH} to the new snapshot version
git fetch origin "${BASEBRANCH}":"${BASEBRANCH}" || true
git checkout "${BASEBRANCH}"

# change VERSION file + commit change into ${BASEBRANCH} branch
if [[ "${BASEBRANCH}" != "${BRANCH}" ]]; then
  # bump the y digit
  [[ $BRANCH =~ ^([0-9]+)\.([0-9]+)\.x ]] && BASE=${BASH_REMATCH[1]}; NEXT=${BASH_REMATCH[2]}; (( NEXT=NEXT+1 )) # for BRANCH=7.10.x, get BASE=7, NEXT=11
  NEXTVERSION="${BASE}.${NEXT}.0-SNAPSHOT"
else
  # bump the z digit
  [[ $VERSION =~ ^([0-9]+)\.([0-9]+)\.([0-9]+) ]] && BASE="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"; NEXT="${BASH_REMATCH[3]}"; (( NEXT=NEXT+1 )) # for VERSION=7.7.1, get BASE=7.7, NEXT=2
  NEXTVERSION="${BASE}.${NEXT}-SNAPSHOT"
fi

# change VERSION file
echo "${NEXTVERSION}" > VERSION
if [[ ${NOCOMMIT} -eq 0 ]]; then
  BRANCH=${BASEBRANCH}
  # commit change into branch
  COMMIT_MSG="[release] Bump to ${NEXTVERSION} in ${BRANCH}"
  git commit -s -m "${COMMIT_MSG}" VERSION
  git pull origin "${BRANCH}"

  PUSH_TRY="$(git push origin "${BRANCH}")"
  # shellcheck disable=SC2181
  if [[ $? -gt 0 ]] || [[ $PUSH_TRY == *"protected branch hook declined"* ]]; then
  PR_BRANCH=pr-main-to-${NEXTVERSION}
    # create pull request for main branch, as branch is restricted
    git branch "${PR_BRANCH}"
    git checkout "${PR_BRANCH}"
    git pull origin "${PR_BRANCH}"
    git push origin "${PR_BRANCH}"
    lastCommitComment="$(git log -1 --pretty=%B)"
    hub pull-request -o -f -m "${lastCommitComment}

${lastCommitComment}" -b "${BRANCH}" -h "${PR_BRANCH}"
  fi
fi
