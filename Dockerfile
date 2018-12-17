FROM golang:alpine

COPY . /go/src/github.com/sourcegraph/go-langserver
RUN go install github.com/sourcegraph/go-langserver
RUN apk add --no-cache openssh git curl
CMD ["go-langserver"]
