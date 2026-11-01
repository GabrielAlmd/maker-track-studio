# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24.14.0-alpine AS web
WORKDIR /src
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build

# Execute the CGO compiler on the target architecture (native runner or QEMU).
FROM golang:1.26.10-alpine AS api
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api/ ./
RUN CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o /server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /selfhostctl ./cmd/selfhostctl

FROM alpine:3.23.7 AS runtime
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tini curl nginx && addgroup -g 10001 maker && adduser -D -u 10001 -G maker maker \
    && mkdir -p /workspace /diagnostics /backups /data/tls \
    && chown -R maker:maker /workspace /diagnostics /backups /data/tls
COPY --from=api /selfhostctl /usr/local/bin/selfhostctl
COPY --from=api /server /usr/local/bin/maker-api
COPY --from=web /src/dist /srv/maker-track/web
COPY LICENSE THIRD_PARTY_FONTS_OFL-1.1.txt /usr/share/licenses/maker-track/
COPY deployments/self-hosted/nginx.conf.template /etc/nginx/maker-track.conf.template
COPY --chmod=755 deployments/self-hosted/start.sh deployments/self-hosted/health.sh /usr/local/bin/
USER 10001:10001
EXPOSE 8443
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["/usr/local/bin/health.sh"]
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/start.sh"]
