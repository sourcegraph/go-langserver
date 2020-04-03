FROM golang:alpine AS builder

RUN apk add --no-cache ca-certificates

ENV CGO_ENABLED=0 GO111MODULE=on
WORKDIR /go/src/github.com/google/zoekt

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go install .

FROM alpine AS go-langserver

RUN apk add --no-cache openssh git curl ca-certificates bind-tools tini

COPY --from=builder /go/bin/* /usr/local/bin/

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["go-langserver"]
