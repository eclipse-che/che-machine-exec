# Copyright (c) 2019 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#
# Contributors:
#   Red Hat, Inc. - initial API and implementation
#

# https://access.redhat.com/containers/?tab=tags#/registry.access.redhat.com/rhel8/go-toolset
FROM rhel8/go-toolset:1.11.13-18 as builder
ENV PATH=/opt/rh/go-toolset-1.11/root/usr/bin:$PATH \
    GOPATH=/go/
USER root
WORKDIR /go/src/github.com/eclipse/che-machine-exec/
COPY . .
RUN adduser unprivilegeduser && \
    CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w -s' -a -installsuffix cgo -o che-machine-exec .

FROM scratch
ENV SUMMARY="Red Hat CodeReady Workspaces machine exec container" \
    DESCRIPTION="Red Hat CodeReady Workspaces machine exec container" \
    PRODNAME="codeready-workspaces" \
    COMPNAME="machineexec-rhel8"
LABEL summary="$SUMMARY" \
      description="$DESCRIPTION" \
      io.k8s.description="$DESCRIPTION" \
      io.k8s.display-name="$DESCRIPTION" \
      io.openshift.tags="$PRODNAME,$COMPNAME" \
      com.redhat.component="$PRODNAME-$COMPNAME-container" \
      name="$PRODNAME/$COMPNAME" \
      version="2.0" \
      license="EPLv2" \
      maintainer="Nick Boldt <nboldt@redhat.com>" \
      io.openshift.expose-services="" \
      usage=""

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /go/src/github.com/eclipse/che-machine-exec/che-machine-exec /go/bin/che-machine-exec

USER unprivilegeduser

ENTRYPOINT ["/go/bin/che-machine-exec"]
