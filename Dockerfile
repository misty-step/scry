FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG BUILD_SHA=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.buildSHA=${BUILD_SHA}" -o /scry ./cmd/scry \
    && mkdir -p /data /backups && chown 65532:65532 /data /backups

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /scry /scry
COPY --from=build --chown=65532:65532 /data /data
COPY --from=build --chown=65532:65532 /backups /backups
USER 65532:65532
ENV PORT=8080 SCRY_MODE=test SCRY_DB=/data/scry.db BACKUP_DIR=/backups
VOLUME ["/data", "/backups"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/scry", "healthcheck"]
ENTRYPOINT ["/scry"]
