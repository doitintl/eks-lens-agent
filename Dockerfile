# syntax = docker/dockerfile:experimental

#
# ----- Go Builder Image ------
#
FROM --platform=${BUILDPLATFORM} golang:1.20-alpine AS builder

# curl git bash
RUN apk add --no-cache curl git bash make ca-certificates

# Create a minimal passwd so we can run as non-root in the container
RUN echo "nobody:x:65534:65534:Nobody:/:" > /etc/passwd.min

#
# ----- Build and Test Image -----
#
FROM --platform=${BUILDPLATFORM} builder AS build

# passed by buildkit
ARG TARGETOS
ARG TARGETARCH

# set working directorydoc
RUN mkdir -p /go/src/app
WORKDIR /go/src/app

# load dependency
COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/go/mod go mod download

# copy sources
COPY . .

# build
RUN make build TARGETOS=${TARGETOS} TARGETARCH=${TARGETARCH}

#
# ------ release Docker image ------
#
FROM scratch

# set user and group
USER 1000

# copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# copy minimal passwd
COPY --from=builder /etc/passwd.min /etc/passwd

# this is the last command since it's never cached
COPY --from=build /go/src/app/.bin/github.com/doitintl/eks-lens-agent /eks-lens-agent

# set user nobody
USER nobody

ENTRYPOINT ["/eks-lens-agent"]