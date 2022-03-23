FROM golang:alpine as builder

RUN apk add --no-cache ca-certificates tzdata

FROM scratch

ARG CREATED
ARG REVISION
ARG VERSION
ARG TITLE
ARG SOURCE
ARG AUTHORS
LABEL org.opencontainers.image.created=$CREATED \
        org.opencontainers.image.revision=$REVISION \
        org.opencontainers.image.title=$TITLE \
        org.opencontainers.image.source=$SOURCE \
        org.opencontainers.image.version=$VERSION \
        org.opencontainers.image.authors=$AUTHORS \
        org.opencontainers.image.vendor="Habx"

ENV TZ=Europe/Paris

WORKDIR /go/src/github.com/habx/aws-mq-cleaner/

COPY --from=builder /usr/share/zoneinfo/ /usr/share/zoneinfo/
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs/
COPY dist/aws-mq-cleaner_linux_amd64/aws-mq-cleaner_linux_amd64 /go/src/github.com/habx/aws-mq-cleaner/aws-mq-cleaner_linux_amd64

ENTRYPOINT ["./aws-mq-cleaner_linux_amd64"]
