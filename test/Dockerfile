FROM golang:1.14

ARG DOCKER_CLI_VERSION="18.09.6"

ENV DOWNLOAD_URL="https://download.docker.com/linux/static/stable/x86_64/docker-$DOCKER_CLI_VERSION.tgz"

RUN mkdir -p /tmp/download \
     && curl -L $DOWNLOAD_URL | tar -xz -C /tmp/download \
     && mv /tmp/download/docker/docker /usr/local/bin/ \
     && rm -rf /tmp/download