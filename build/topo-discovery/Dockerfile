# SPDX-FileCopyrightText: 2022-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0

FROM onosproject/golang-build:v1.3.0 as build

ENV GO111MODULE=on
WORKDIR /build

# copy only the files that are needed for the build, exclude everything else to make better use of the docker cache
COPY ./cmd /build/cmd
COPY ./pkg /build/pkg
COPY ./vendor /build/vendor
COPY ./go.mod /build
COPY ./go.sum /build

# build the executable
RUN go build -mod=vendor -o build/_output/topo-discovery ./cmd/topo-discovery

FROM alpine:3.13
RUN apk add libc6-compat

USER nobody

COPY --from=build /build/build/_output/topo-discovery /usr/local/bin/topo-discovery

ENTRYPOINT ["topo-discovery"]