# Copyright 2019 Iguazio
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

FROM gcr.io/iguazio/golang:1.17

ARG DOCKER_CLI_VERSION="18.09.6"

ENV DOWNLOAD_URL="https://download.docker.com/linux/static/stable/x86_64/docker-$DOCKER_CLI_VERSION.tgz"

RUN mkdir -p /tmp/download \
     && curl -L $DOWNLOAD_URL | tar -xz -C /tmp/download \
     && mv /tmp/download/docker/docker /usr/local/bin/ \
     && rm -rf /tmp/download
