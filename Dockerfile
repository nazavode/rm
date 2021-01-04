# syntax = docker/dockerfile:1.2

FROM --platform=${BUILDPLATFORM} golang:1.15.6-alpine AS base
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

FROM base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/rmd ./cmd/rmd

FROM scratch AS bin-unix
COPY --from=build /out/rmd /

FROM bin-unix AS bin-linux
FROM bin-unix AS bin-darwin

FROM scratch AS bin-windows
COPY --from=build /out/rmd /rmd.exe

FROM bin-${TARGETOS} as bin

# No pandoc package available in alpine yet
#
# FROM --platform=${BUILDPLATFORM} alpine:latest AS deploy
# COPY --from=bin /rmd /rmd
# RUN apk update \
#  && apk upgrade \
#  && apk add pandoc ca-certificates \
#  && rm -rf /var/cache/apk/* \
#  && update-ca-certificates

FROM --platform=${BUILDPLATFORM} ubuntu:20.04 AS deploy
COPY --from=bin /rmd /rmd
RUN apt-get -yqq update && apt-get -yqq upgrade \
 && apt-get -yqq install pandoc ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && update-ca-certificates
ENTRYPOINT ["/rmd"]