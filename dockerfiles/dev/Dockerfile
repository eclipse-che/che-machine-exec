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
# Dockerfile defines che-machine-exec development image eclipse/che-machine-exec-dev
#

FROM quay.io/eclipse/che-golang-1.10:nightly

ARG DEP_LOCATION=/go/bin/dep

RUN wget https://github.com/golang/dep/releases/download/v0.5.1/dep-linux-amd64 -O ${DEP_LOCATION} && \
    chmod +x ${DEP_LOCATION}
EXPOSE 5555 5555
