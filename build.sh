#!/bin/bash


# buildah build-using-dockerfile -t  docker.io/aandrienko/che-machine-exec:token  .
# buildah push docker.io/aandrienko/che-machine-exec:token

docker build -t  docker.io/aandrienko/che-machine-exec:token2  .
docker push docker.io/aandrienko/che-machine-exec:token2