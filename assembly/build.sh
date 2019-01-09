#!/bin/sh
set -e
set -u

if [ -f che-service-plugin.tar.gz ]; then
    rm che-service-plugin.tar.gz
fi

cd etc
tar uvf ../che-service-plugin.tar .
cd ..
gzip che-service-plugin.tar
