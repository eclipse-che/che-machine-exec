#!/bin/bash
# Release process automation script. 
# Used to create branch/tag, update VERSION files and and trigger release by force pushing changes to the release branch 

# set to 1 to actually trigger changes in the release branch
TRIGGER_RELEASE=0 

#REPO=git@github.com:eclipse/che-machine-exec
#VERSION=7.7.0

while [[ "$#" -gt 0 ]]; do
  case $1 in
    '-t'|'--trigger-release') TRIGGER_RELEASE=1; shift 0;;
	'-r'|'--repo') REPO="$2"; shift 1;;
    '-v'|'--version') VERSION="$2"; shift 1;;
  esac
  shift 1
done

usage ()
{
    echo "Usage: $0 --repo [GIT REPO TO EDIT] --version [VERSION TO RELEASE] [--trigger-release]"
    echo "Example: $0 --repo git@github.com:eclipse/che-machine-exec --version 7.7.0 --trigger-release"
}
if [[ ! ${VERSION} ]] || [[ ! ${REPO} ]]; then
  usage
  exit 1
fi

BRANCH=${VERSION%.*}.x

TMP=$(mktemp -d); mkdir -p $TMP; cd $TMP 

# get sources
echo "Check out ${REPO} to ${TMP}/${REPO##*/}"
git clone ${REPO} -q
cd ${REPO##*/}
git fetch origin master:master
git checkout master

# create new branch
git branch ${BRANCH} || git checkout ${BRANCH} && git pull origin ${BRANCH}
git push origin ${BRANCH}
git fetch origin ${BRANCH}:${BRANCH}
git checkout ${BRANCH}

# change VERSION file + commit change
echo ${VERSION} > VERSION
git commit -s -m "[release] Bump to ${VERSION} in ${BRANCH}" VERSION
git pull origin ${BRANCH}
git push origin ${BRANCH}

# push new branch to release branch
if [[ $TRIGGER_RELEASE -eq 1 ]]; then
    git fetch origin ${BRANCH}:${BRANCH}
    git checkout ${BRANCH}
    git branch release -f 
    git push origin release -f

    git checkout ${BRANCH}
    git tag ${VERSION}
    git push origin ${VERSION}
fi

# now update master to the new snapshot version
git fetch origin master:master
git checkout master

# change VERSION file + commit change
[[ $BRANCH =~ ^([0-9]+)\.([0-9]+).x ]] && BASE=${BASH_REMATCH[1]}; NEXT=${BASH_REMATCH[2]}; let NEXT=NEXT+1 # for BRANCH=7.10.x, get BASE=7, NEXT=11
echo "${BASE}.${NEXT}.0-SNAPSHOT" > VERSION
BRANCH=master
git commit -s -m "[release] Bump to ${BASE}.${NEXT}.0-SNAPSHOT in ${BRANCH}" VERSION
git pull origin ${BRANCH}
git push origin ${BRANCH}

# cleanup temp
cd /tmp && rm -fr $TMP

