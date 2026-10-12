FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS builder

ARG GOPROXY=https://proxy.golang.org,direct
ARG TARGETOS=linux
ARG TARGETARCH
ARG APP_COMMAND
ENV GOPROXY=${GOPROXY}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN case "${APP_COMMAND}" in api|worker|migrate) ;; *) echo "APP_COMMAND must be api, worker, or migrate" >&2; exit 2 ;; esac \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false -o /app ./cmd/${APP_COMMAND}

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
ARG APP_COMMAND
ARG SOURCE_COMMIT=unknown
LABEL org.opencontainers.image.title="distributed-task-lab-${APP_COMMAND}" \
      org.opencontainers.image.source="https://github.com/QuietYouthKept/distributed-task-lab" \
      org.opencontainers.image.revision="${SOURCE_COMMIT}" \
      org.opencontainers.image.description="Local-first production-like tasklab ${APP_COMMAND} binary"
RUN apk add --no-cache ca-certificates \
    && addgroup -S tasklab \
    && adduser -S -G tasklab -h /app tasklab
COPY --from=builder --chown=tasklab:tasklab /app /app/tasklab
USER tasklab
ENTRYPOINT ["/app/tasklab"]
