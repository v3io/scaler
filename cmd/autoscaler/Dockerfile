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
