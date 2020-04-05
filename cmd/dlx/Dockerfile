FROM golang:1.14-alpine3.11 AS builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /dlx

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o dlx cmd/dlx/main.go

FROM alpine:3.11

COPY --from=builder /dlx/dlx /home/v3io/bin/dlx
