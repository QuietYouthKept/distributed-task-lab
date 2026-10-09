# Phase 1 Wave 2 — 首次远程 CI 故障修复记录

本记录对应首次远程验收的失败 Run：

- CI：[run 37956337334](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37956337334)
- Security：[run 37956336466](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37956336466)
- Pull Request：[PR #1](https://github.com/QuietYouthKept/distributed-task-lab/pull/1)

这是一份修复说明，不是新的 Wave 2 Evidence。历史 Run、冻结 Implementation Commit 上的 14/14 本地 Evidence 及 Evidence 索引均未改写，也没有把旧结果重新归属到新代码。

## MinIO CI

### 根因

失败 Job 将本地 Docker 镜像 ID `sha256:1dce…` 写成 `minio/minio@sha256:…` Registry Digest。全新 GitHub Runner 将其解释为 Docker Hub Registry Digest，实际请求失败为 `pull access denied for minio/minio`，Docker 退出码 125。此前的成功仅能证明某台开发机有该镜像缓存，不能证明公开 Registry 可按该 Digest 拉取。

### 修复方案与来源

不再拉取原镜像，也不依赖开发机缓存或 CI Registry 凭据。CI 每次运行都通过 Go Module Proxy 获取并校验官方 MinIO 源码模块，再在 Runner 上构建临时 Linux/amd64 镜像：

- 上游仓库：[minio/minio](https://github.com/minio/minio)，许可证为 AGPL-3.0；仓库当前已归档，因此该 MinIO 仅用于兼容性集成测试，不应被理解为项目的生产存储服务或获得上游维护承诺。
- 上游发布标签：`RELEASE.2025-10-15T17-29-55Z`。
- 固定源码提交：`9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`。
- 固定 Go 模块伪版本：`github.com/minio/minio@v0.0.0-20251015172955-9e49d5e7a648`，伪版本中的提交前缀与完整 Git 提交一致。Go 工具通过公共模块代理及校验数据库验证模块内容；构建产物中包含上游 `LICENSE`。
- CI 构建脚本再固定并比较模块 zip checksum `h1:6TdolSCLSs2nwm8i0PpWDqf9iX2Ty9WQK8wmr7dCnUM=` 与 `go.mod` checksum `h1:yCWDkwWO9IWpGsT4mreDDN/B/QVmK2zC666uInRAcqE=`，把两项校验值和上游提交写入镜像标签。
- 镜像配方：`.github/minio-ci.Dockerfile`；构建脚本：`scripts/evidence/build-minio-ci-image.sh`。构建使用固定的 Go `1.26.9`、`CGO_ENABLED=0`、`linux/amd64`、`-trimpath`，并禁用 Docker layer cache。

此方案没有发布镜像到 GHCR 或其他 Registry，因此没有 Registry Digest 可报告；脚本明确将构建机本地 Docker Image ID 标为“not a registry digest”。无需新增 `packages: write` 权限、上传镜像或新增用户授权。真实远程 Runner 的源码获取、构建及 Docker Engine 行为，需推送后由新 CI Run 最终确认。

Job 保留随机临时 MinIO root credentials，以权限受限临时文件传给容器；Credential 值注册为 Actions mask，S3 endpoint 绑定到 loopback，数据放在容器 tmpfs。健康检查最多等待 60 秒。失败时收集当前 Job 的 MinIO 容器日志并对两项随机凭据做替换；不使用 `--rm`，确保失败日志在清理前仍可读取。`always()` 清理步骤只删除本 Job Run ID / Attempt 命名的容器及临时镜像标签。

以下必需验证仍在 `.github/workflows/ci.yml` 中执行，没有 Mock、Skip 或降级为可选 Job：真实 S3 Contract、API + 两个独立 Worker 的 Crash Recovery、PostgreSQL Backup/Restore。

## govulncheck

### 根因

原 `go-package: ./cmd/... ./internal/... ./pkg/...` 被 `golang/govulncheck-action` 当成单个包模式传递，导致 `no packages matched the provided patterns`。Action 的 `go-version-file` 又与默认的 `go-version: stable` 同时生效，原 Go `1.25.0` 项目在该 Run 中实际使用了 Go `1.27.2`。

### 修复及漏洞处置

- Action 改用单个 `go-package: ./...`，覆盖根 Go Module 下全部包。
- Action 改用单一明确版本输入 `go-version-input: "1.26.9"`；根 `go.mod` 的 Go 版本同步为 `1.26.9`，避免 Action 默认 stable 漂移，并纳入当时扫描到 Go 标准库漏洞的修复版本。
- 修复前实际扫描不止参数错误：识别到 13 项可达漏洞，涉及 Go 标准库、`golang.org/x/net`、OpenTelemetry 及 gRPC。已分别更新 Go 工具链标准库、`x/net`、OpenTelemetry 1.45.0、gRPC 1.83.2 和 `klauspost/compress` 1.18.7；后两个是在 verbose 模块结果中发现修复版本后补齐的。
- 修复后本机运行 `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -format text ./...`：0 项代码可达漏洞、0 项被导入包漏洞；verbose 另报告 `GO-2026-5932`：`golang.org/x/crypto/openpgp` 存在于依赖图中，但项目代码未调用受影响包，且该 Advisory 没有修复版本。没有禁用、忽略或屏蔽这项检查。

更新依赖后，全量测试暴露了一个实际 OTLP 配置问题：Trace exporter 把标准 `OTEL_EXPORTER_OTLP_ENDPOINT` 基础 URL 原样作为完整 signal URL，向 `/` 发请求。`internal/observability/tracing.go` 现在保留 base path 并追加 `/v1/traces`；既有接收器测试验证导出请求的真实路径、Trace 与 service name。

## 本地验证

以下验证在 Windows 本机完成，使用固定来源构建的真实 MinIO 镜像及本地 PostgreSQL 16 容器；不是 GitHub hosted runner 验证，也不是历史 Evidence 的重新测量：

- MinIO S3 Contract + Race：通过。
- API + 两 Worker Crash Recovery + Race：通过；日志确认存活 Worker 以 `attempt=2`、`lease_version=2` 恢复共享 Artifact。
- PostgreSQL Backup/Restore Drill + Race：通过；恢复的 Task、Effects、Audit、Events、Schema Migration 计数及引用 Artifact 均一致。
- `go test ./cmd/... ./internal/... ./pkg/... -count=1`：通过，使用 `REQUIRE_TEST_DATABASE=1`。
- `go test -race ./cmd/... ./internal/... ./pkg/... -count=1`：通过，使用 `REQUIRE_TEST_DATABASE=1`。
- Govulncheck `./...`：0 项可达漏洞；保留上述 1 项不可达且无修复版本的模块 Advisory。

GitHub Actions 远程复验尚未发生。需将本地修复 Commit 推送到 PR 分支后再检查新的 CI 与 Security Run；旧失败 Run 必须继续保留作为基线证据。
