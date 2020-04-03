FROM golang:alpine

COPY . /go/src/github.com/sourcegraph/go-langserver
RUN cd /go/src/github.com/sourcegraph/go-langserver && go install .
RUN apk add --no-cache openssh git curl
CMD ["go-langserver"]
