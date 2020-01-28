## Major / Minor Release

Below are the steps needed to do a release. But rather than doing them by hand, you can run this script:

https://github.com/eclipse/che-machine-exec/blob/master/make-release.sh

- create a branch for the release e.g. `7.6.x`
- provide a [PR](https://github.com/eclipse/che-machine-exec/pull/66) with bumping the [VERSION](https://github.com/eclipse/che-machine-exec/blob/master/VERSION) file to the `7.6.x` branch (changing `7.6.0-SNAPSHOT` to `7.6.0`)
- [![Release Build Status](https://ci.centos.org/buildStatus/icon?subject=release&job=devtools-che-machine-exec-release/)](https://ci.centos.org/job/devtools-che-machine-exec-release/) CI is triggered based on the changes in the [release](https://github.com/eclipse/che-machine-exec/tree/release) branch (not `7.6.x`).

In order to trigger the CI once the [PR](https://github.com/eclipse/che-machine-exec/pull/66) is merged to the `7.6.x` one needs to:

```
 git fetch origin 7.6.x:7.6.x
 git checkout 7.6.x
 git branch release -f 
 git push origin release -f
```

[CI](https://ci.centos.org/job/devtools-che-machine-exec-release/) will build an image from the [`release`](https://github.com/eclipse/che-machine-exec/tree/release) branch and push it to [quay.io](https://quay.io/organization/eclipse) e.g [quay.io/eclipse/che-machine-exec:7.6.0](https://quay.io/repository/eclipse/che-machine-exec?tab=tags&tag=7.6.0)

The last thing is the tag `7.6.0` creation from the `7.6.x` branch

```
git checkout 7.6.x
git tag 7.6.0
git push origin 7.6.0
```

After the release, the `VERSION` file should be bumped in the master e.g. [`7.7.0-SNAPSHOT`](https://github.com/eclipse/che-machine-exec/pull/67)

## Service / Bugfix  Release

The release process is the same as for the Major / Minor one, but the values passed to the `make-release.sh` script will differ so that work is done in the existing 7.7.x branch.

```
./make-release.sh --repo git@github.com:eclipse/che-machine-exec --version 7.7.1 --trigger-release
```

