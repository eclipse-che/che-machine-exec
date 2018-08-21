#!/bin/bash
#
# Copyright (c) 2012-2017 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#

function resolveDependencies() {
    echo "===>Resolve go-lang dependencies with help dep tool<===";
    dep ensure
      if [ $? != 0 ]; then
        echo "Failed to resolve dependencies";
        exit 0;
    fi
}

function compile() {
    resolveDependencies;

    echo "===>Compile che-machine-exec binary from source code.<===";

    $(CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w -s' -a -installsuffix cgo -o che-machine-exec .);

    if [ $? != 0 ]; then
        echo "Failed to compile code";
        exit 0;
    fi

    echo "============Compilation succesfully completed.============";
}

compile;
