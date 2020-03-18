#!/bin/bash


buildah build-using-dockerfile -t  docker.io/aandrienko/che-machine-exec:token2 .
buildah push docker.io/aandrienko/che-machine-exec:token2

# docker build -t docker.io/aandrienko/che-machine-exec:token2  .
# docker push docker.io/aandrienko/che-machine-exec:token2

# docker build -t docker.io/aandrienko/che-machine-exec:nightly  .
# docker push docker.io/aandrienko/che-machine-exec:nightly