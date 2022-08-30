FROM gcr.io/iguazio/golang:1.17-alpine3.15 AS builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /dlx

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o dlx cmd/dlx/main.go

FROM gcr.io/iguazio/alpine:3.16

COPY --from=builder /dlx/dlx /home/v3io/bin/dlx
