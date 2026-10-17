FROM debian:trixie@sha256:913f6706df59a68922d1dd08f78c2476560a8d367897200a6005b00e5f67c2d5

# Versions (override at build time if desired)
ARG GO_VERSION=1.26.9
ARG GO_AMD64_SHA256=42d158b4d8f7b61ac0a830567c940a86098fb7aac52e467a5ebec03ef5cc2f8d
ARG GO_ARM64_SHA256=4a97373d49fcacdcf3694fea368a500b00ee3e963974f3e7514132717632f052

ARG BUILDX_VERSION=0.38.0
ARG COMPOSE_VERSION=5.6.0
ARG PLUGIN_X_NET_VERSION=v0.61.0
ARG PLUGIN_GO_ARCHIVE_VERSION=v0.3.3

# Apply security fixes to inherited packages before adding Docker Engine dependencies.
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    iptables \
    uidmap \
    fuse-overlayfs \
    && rm -rf /var/lib/apt/lists/*

# -------------------------------------------------------------
# Install Docker Engine (client + dockerd)
# -------------------------------------------------------------
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg \
      -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo \
      "deb [arch=$(dpkg --print-architecture) \
      signed-by=/etc/apt/keyrings/docker.gpg] \
      https://download.docker.com/linux/debian \
      $(. /etc/os-release; echo $VERSION_CODENAME) stable" \
      > /etc/apt/sources.list.d/docker.list

RUN apt-get update && apt-get install -y --no-install-recommends \
    docker-ce \
    docker-ce-cli \
    containerd.io \
    && rm -rf /var/lib/apt/lists/*

RUN ln -s /usr/libexec/docker/cli-plugins/docker-compose /usr/local/bin/docker-compose

# -------------------------------------------------------------
# Install Go
# -------------------------------------------------------------
RUN arch="$(dpkg --print-architecture)" && \
    case "$arch" in \
      amd64) checksum="$GO_AMD64_SHA256" ;; \
      arm64) checksum="$GO_ARM64_SHA256" ;; \
      *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o /tmp/go.tgz && \
    echo "${checksum}  /tmp/go.tgz" | sha256sum -c - && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:/usr/local/go/bin:${PATH}"

RUN mkdir -p /go && chmod -R 777 /go

COPY build-plugins.sh /tmp/build-plugins.sh
RUN --mount=type=cache,target=/tmp/gocache \
    --mount=type=cache,target=/go/pkg/mod \
    GOCACHE=/tmp/gocache sh /tmp/build-plugins.sh && rm /tmp/build-plugins.sh

# -------------------------------------------------------------
# Docker-in-Docker setup
# -------------------------------------------------------------
# Default working directory for Docker
RUN mkdir -p /var/lib/docker

# Standard DinD entrypoint (similar to docker:dind)
COPY --from=docker:27-dind@sha256:aa3df78ecf320f5fafdce71c659f1629e96e9de0968305fe1de670e0ca9176ce /usr/local/bin/dind /usr/local/bin/dind
COPY --from=docker:27-dind@sha256:aa3df78ecf320f5fafdce71c659f1629e96e9de0968305fe1de670e0ca9176ce /usr/local/bin/docker-init /usr/local/bin/docker-init

# Make dockerd the default entrypoint
ENTRYPOINT ["/usr/local/bin/dind"]

CMD ["dockerd"]
