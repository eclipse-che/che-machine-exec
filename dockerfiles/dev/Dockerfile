#
# Copyright (c) 2018-2019 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#
# Contributors:
#   Red Hat, Inc. - initial API and implementation
#

FROM golang:1.10.3-alpine

ARG DEP_LOCATION=/go/bin/dep

RUN apk add --update git bash wget && \
    wget https://github.com/golang/dep/releases/download/v0.5.1/dep-linux-amd64 -O ${DEP_LOCATION} && \
    chmod +x ${DEP_LOCATION}

RUN apk del wget

EXPOSE 5555 5555

WORKDIR /projects/src

CMD tail -f /dev/null
