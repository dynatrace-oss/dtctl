FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY dtctl /usr/local/bin/dtctl

ENTRYPOINT ["/usr/local/bin/dtctl"]
CMD ["--help"]
