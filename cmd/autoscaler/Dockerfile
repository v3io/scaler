FROM golang:1.12-rc-alpine3.8

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /go/src/github.com/v3io/scaler
COPY . .

RUN go get github.com/v3io/scaler-types

RUN mkdir -p /home/v3io/bin \
            && GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o /home/v3io/bin/autoscaler cmd/autoscaler/main.go \
            && rm -rf /go/src/github/v3io/scaler
