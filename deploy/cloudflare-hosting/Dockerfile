# Scry on Cloudflare Containers: the exact operator release binary plus a local
# nginx ingress that reproduces the private exe.dev trust position (single
# canonical Host, exact owner identity header, loopback-only application).
# Container disk is ephemeral; durable state lives in the private R2 recovery
# sink and is restored on every instance start (see entrypoint.sh).
FROM alpine:3.20

# nginx: local ingress only. curl: snapshot fetch from the private gateway.
# ca-certificates: HTTPS to *.workers.dev / scry.study gateways.
RUN apk add --no-cache nginx curl ca-certificates && \
    addgroup -g 1000 -S scry && adduser -u 1000 -S -G scry -h /var/lib/scry -s /sbin/nologin scry && \
    mkdir -p /var/lib/scry/data /var/lib/scry/backups /var/lib/scry/restore && \
    chown -R scry:scry /var/lib/scry && \
    mkdir -p /run/nginx && chown scry:scry /run/nginx

COPY scry /usr/local/bin/scry
COPY nginx.conf /etc/nginx/nginx.conf
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/scry /usr/local/bin/entrypoint.sh /etc/nginx/nginx.conf && \
    /usr/local/bin/scry version

USER scry
WORKDIR /var/lib/scry
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
