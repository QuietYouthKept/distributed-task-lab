FROM scratch

ARG MINIO_SOURCE_TAG
ARG MINIO_SOURCE_COMMIT
ARG MINIO_MODULE_VERSION
ARG MINIO_MODULE_SUM

LABEL org.opencontainers.image.source="https://github.com/minio/minio" \
      org.opencontainers.image.revision="${MINIO_SOURCE_COMMIT}" \
      org.opencontainers.image.version="${MINIO_SOURCE_TAG}" \
      org.opencontainers.image.licenses="AGPL-3.0" \
      com.tasklab.minio.go-module-sum="${MINIO_MODULE_SUM}" \
      org.opencontainers.image.description="Ephemeral CI-only MinIO built from pinned upstream source"

COPY minio /usr/bin/minio
COPY LICENSE /usr/share/licenses/minio/LICENSE

EXPOSE 9000 9001
ENTRYPOINT ["/usr/bin/minio"]
