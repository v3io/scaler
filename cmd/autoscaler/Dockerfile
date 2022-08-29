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
FROM golang:1.14.3-alpine3.11 AS builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /autoscaler

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o autoscaler cmd/autoscaler/main.go

FROM alpine:3.11

COPY --from=builder /autoscaler/autoscaler /home/v3io/bin/autoscaler
