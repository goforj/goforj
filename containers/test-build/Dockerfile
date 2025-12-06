FROM debian:trixie

# Versions (override at build time if desired)
ARG GO_VERSION=1.25.0

# Install base deps + Docker Engine dependencies
RUN apt-get update && apt-get install -y \
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

RUN apt-get update && apt-get install -y \
    docker-ce \
    docker-ce-cli \
    containerd.io \
    docker-buildx-plugin \
    docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

RUN ln -s /usr/libexec/docker/cli-plugins/docker-compose /usr/local/bin/docker-compose

# -------------------------------------------------------------
# Install Go
# -------------------------------------------------------------
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz -o /tmp/go.tgz && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:/usr/local/go/bin:${PATH}"

RUN mkdir -p /go && chmod -R 777 /go

# -------------------------------------------------------------
# Docker-in-Docker setup
# -------------------------------------------------------------
# Default working directory for Docker
RUN mkdir -p /var/lib/docker

# Standard DinD entrypoint (similar to docker:dind)
COPY --from=docker:27-dind /usr/local/bin/dind /usr/local/bin/dind
COPY --from=docker:27-dind /usr/local/bin/docker-init /usr/local/bin/docker-init

# Make dockerd the default entrypoint
ENTRYPOINT ["/usr/local/bin/dind"]

CMD ["dockerd"]
