# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-build
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend-build
ARG GOPROXY=https://goproxy.cn,direct
ARG APP_VERSION=dev
ARG VCS_REF=unknown
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
COPY VERSION ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./backend/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    RESOLVED_VERSION="${APP_VERSION}"; \
    if [ "${RESOLVED_VERSION}" = "dev" ]; then RESOLVED_VERSION="$(tr -d '\r\n' < VERSION)"; fi; \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.appVersion=${RESOLVED_VERSION} -X main.buildCommit=${VCS_REF}" \
    -o /out/classorbit ./backend

FROM alpine:3.22
ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
RUN sed -i -E "s#https?://[^/]*/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata \
    && addgroup -S classorbit \
    && adduser -S -G classorbit classorbit \
    && mkdir -p /app/data /app/public \
    && chown -R classorbit:classorbit /app
WORKDIR /app
COPY --from=backend-build --chown=classorbit:classorbit /out/classorbit /app/classorbit
COPY --from=frontend-build --chown=classorbit:classorbit /src/frontend/dist/ /app/public/
USER classorbit
ENV ADDR=0.0.0.0:8080 \
    DATA_DIR=/app/data \
    PUBLIC_DIR=/app/public \
    TZ=Asia/Shanghai
EXPOSE 8080
CMD ["/app/classorbit"]
