#
# Copyright (c) 2012-2019 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#
# Contributors:
#   Red Hat, Inc. - initial API and implementation
#
# Dockerfile defines che-machine-exec production image eclipse/che-machine-exec-dev
#

FROM golang:1.10.3-alpine as builder
WORKDIR /go/src/github.com/eclipse/che-machine-exec/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w -s' -a -installsuffix cgo -o che-machine-exec .
RUN apk add --no-cache ca-certificates

RUN adduser -D -g '' unprivilegeduser && \
    mkdir -p /rootfs/tmp /rootfs/etc /rootfs/etc/ssl/certs /rootfs/go/bin && \
    # In the `scratch` you can't use Dockerfile#RUN, because there is no shell and no standard commands (mkdir and so on).
    # That's why prepare absent in the scratch /tmp folder.
    chmod 1777 /rootfs/tmp && \
    cp -rf /etc/passwd /rootfs/etc && \
    cp -rf /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs && \
    cp -rf /go/src/github.com/eclipse/che-machine-exec/che-machine-exec /rootfs/go/bin

FROM scratch

COPY --from=builder /rootfs /

USER unprivilegeduser

ENTRYPOINT ["/go/bin/che-machine-exec"]
