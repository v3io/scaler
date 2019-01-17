FROM golang:1.10-alpine3.7

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /go/src/github.com/v3io/scaler
COPY . .

# remove vendor, this will be the implemnter job to get it before build
RUN rm -rf vendor

# if build passes a resource scaler go file
ONBUILD ARG RESOURCE_SCALER_GO_FILE=./pkg/resourcescaler/resourcescaler.go

# copy the vendor dir, instead of the one we removed earlier
ONBUILD ARG VENDOR_DIR="vendor"

# run any command the user wished before go build, `:` means nop
ONBUILD ARG BEFORE_BUILD=":"

ONBUILD COPY ${VENDOR_DIR} /go/src/github.com/v3io/scaler/vendor
ONBUILD COPY ${RESOURCE_SCALER_GO_FILE} /go/src/github.com/v3io/scaler/pkg/resourcescaler/resourcescaler.go

ONBUILD RUN ${BEFORE_BUILD}

ONBUILD RUN mkdir -p /home/v3io/bin \
            && GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o /home/v3io/bin/dlx cmd/dlx/main.go \
            && rm -rf /go/src/github/v3io/scaler

CMD ["/home/v3io/bin/dlx"]
