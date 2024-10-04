FROM alpine:latest as certs
RUN apk update && apk add ca-certificates

FROM cgr.dev/chainguard/busybox:latest
COPY --from=certs /etc/ssl/certs /etc/ssl/certs

COPY mango /usr/bin/mango
COPY mh /usr/bin/mh
ENTRYPOINT ["/usr/bin/mango"]
CMD ["--inventory.path", "/opt/mango/inventory"]
