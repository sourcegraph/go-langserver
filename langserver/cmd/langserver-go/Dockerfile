# Docker image for langserver-go

FROM alpine:3.4
MAINTAINER Quinn Slack <sqs@sourcegraph.com>
RUN apk add --no-cache --repository http://dl-cdn.alpinelinux.org/alpine/edge/community ca-certificates
ADD langserver-go /langserver-go
ENTRYPOINT ["/langserver-go"]
