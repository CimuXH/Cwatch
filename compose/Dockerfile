ARG IMAGE_PREFIX
FROM ${IMAGE_PREFIX}golang:1.24.9-bookworm AS build
ARG GOPROXY=https://proxy.golang.org
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=550MiB
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go build -p=1 -mod=readonly -trimpath -ldflags="-s -w" -o /out/cwatch .

FROM ${IMAGE_PREFIX}ubuntu:24.04
ARG APT_MIRROR
RUN if [ -n "$APT_MIRROR" ]; then sed -i "s|http://archive.ubuntu.com/ubuntu|$APT_MIRROR|g; s|http://security.ubuntu.com/ubuntu|$APT_MIRROR|g" /etc/apt/sources.list.d/ubuntu.sources; fi \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && install -d -o 10001 -g 10001 -m 0750 /var/lib/cwatch/media
WORKDIR /opt/cwatch
COPY --from=build /out/cwatch /usr/local/bin/cwatch
COPY web/ /opt/cwatch/web/
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/cwatch"]
