# syntax=docker/dockerfile:1

# Reproducible Linux amd64/arm64 product image for strmgen.
#
# Both base images are pinned by digest. The build stage cross-compiles from the
# host architecture ($BUILDPLATFORM) with CGO_ENABLED=0, so the binary is a
# single static executable for the requested $TARGETARCH. The Go version comes
# from the repository .go-version (1.27.2) and the runtime dependency set comes
# from go.mod/go.sum, which are copied and verified before any source.
#
# The runtime stage contains no architecture-specific RUN step: the base image
# supplies the target busybox, and CA roots plus the unprivileged account files
# are copied as plain data. This keeps the arm64 image buildable on an amd64
# host without QEMU while the final image is genuinely linux/arm64.
#
# The image never contains a token, a real configuration, host output, SQLite
# state, test hooks or the source tree.

FROM --platform=$BUILDPLATFORM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS build

ARG TARGETOS
ARG TARGETARCH

# The automatic platform ARGs are supplied by BuildKit. Fail loudly instead of
# silently producing a host-architecture binary if they are ever absent.
RUN test -n "${TARGETARCH}" && test -n "${TARGETOS}" \
 && test "${TARGETOS}" = "linux"

ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local \
    GOFLAGS=-mod=mod

WORKDIR /src

# Dependency layer: only the module files. go.sum is verified before the build
# so the recorded dependency graph cannot silently change.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY cmd ./cmd
COPY internal ./internal

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w" -o /out/strmgen ./cmd/strmgen

# Preparation stage: creates the unprivileged account and the default data
# directories. It runs on the build platform only to author plain files and
# directory modes; nothing from it is executed in the target image.
FROM --platform=$BUILDPLATFORM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc AS prep

RUN addgroup -S -g 10001 strmgen \
 && adduser -S -u 10001 -G strmgen -H -s /sbin/nologin strmgen \
 && mkdir -p /etc/strmgen /state /output /tmp/strmgen-locks \
 && chown 10001:10001 /state /output /tmp/strmgen-locks \
 && chmod 0755 /tmp/strmgen-locks

FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc AS runtime

# Recorded so the image can be tied back to a source revision and target
# architecture without baking the build context into a layer.
ARG TARGETARCH
ARG SOURCE_REVISION=unknown

LABEL org.opencontainers.image.title="strmgen" \
      org.opencontainers.image.description="OpenList to local STRM generator" \
      org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.base.name="alpine:3.20" \
      com.strmgen.target-arch="${TARGETARCH}"

# CA roots for HTTPS OpenList endpoints. The unprivileged account files and the
# default data directories (owned by the runtime UID) are copied as data only.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=prep /etc/passwd /etc/passwd
COPY --from=prep /etc/group /etc/group
COPY --from=prep --chown=10001:10001 /etc/strmgen /etc/strmgen
COPY --from=prep --chown=10001:10001 /state /state
COPY --from=prep --chown=10001:10001 /output /output
COPY --from=prep --chown=10001:10001 /tmp/strmgen-locks /tmp/strmgen-locks
COPY --from=build /out/strmgen /usr/local/bin/strmgen

# The runtime account is unprivileged and has no shell. The default command is
# the long-running serve entry with a read-only mounted configuration; an
# explicit command override (for example `sync`) keeps the same data mounts.
USER 10001:10001
WORKDIR /

ENTRYPOINT ["/usr/local/bin/strmgen"]
CMD ["serve", "--config", "/etc/strmgen/config.yaml"]
