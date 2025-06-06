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

FROM gcr.io/iguazio/golang:1.23.8-alpine3.18 AS builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /dlx

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o dlx cmd/dlx/main.go

FROM gcr.io/iguazio/alpine:3.18

COPY --from=builder /dlx/dlx /home/v3io/bin/dlx
