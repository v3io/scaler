FROM golang:1.10-alpine3.7

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /go/src/github.com/v3io/scaler
COPY . .

# if build passes a resource scaler go file
ONBUILD ARG RESOURCE_SCALER_GO_FILE=./pkg/resourcescaler/resourcescaler.go

ONBUILD COPY ${RESOURCE_SCALER_GO_FILE} /go/src/github.com/v3io/scaler/pkg/resourcescaler/resourcescaler.go

ONBUILD RUN mkdir -p /home/v3io/bin \
            && GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o /home/v3io/bin/dlx cmd/dlx/main.go \
            && rm -rf /go/src/github/v3io/scaler

CMD ["/home/v3io/bin/dlx"]
