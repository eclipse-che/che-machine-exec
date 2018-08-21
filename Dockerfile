#
# Copyright (c) 2012-2018 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#
# Contributors:
#   Red Hat, Inc. - initial API and implementation
#

FROM golang:1.10.3 as builder

WORKDIR /go/src/github.com/eclipse/che-machine-exec/
COPY . .
RUN go get -u github.com/golang/dep/cmd/dep && \
    ./compile.sh

FROM registry.centos.org/centos:7
COPY --from=builder /go/src/github.com/eclipse/che-machine-exec/che-machine-exec /usr/local/bin
ENTRYPOINT ["che-machine-exec"]
