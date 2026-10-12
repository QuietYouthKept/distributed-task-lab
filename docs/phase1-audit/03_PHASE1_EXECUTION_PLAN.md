# Phase 1 execution plan

本计划建立在 `01_TASK_001_042_STATUS.md` 的只读审计上，仅提出施工，不在本轮实现。范围遵循原 42-task dependency graph；AI Track 默认延后，不把确定性 simulation 包装成模型质量。

## 执行原则与入口门

先保留 Phase 0 tag/evidence 和历史失败证据；每个施工切片先写 invariant、失败状态和回滚，再改实现。每个 phase commit 需要独立测试和新 Evidence ID/manifest，旧 run 永不覆盖。Evidence 需绑定准确 commit SHA、dirty state、环境 fingerprint、命令/退出码、原始日志；凡实际依赖外部基础设施，CI 必须启动可控服务或标为未验证。

施工前有一个产品决策门（不是新 TASK ID）：确认 single-tenant internal vs multi-tenant/public，确定 identity issuer/claims 和 S3 target/provider。身份决策阻塞 TASK-006/007/008；不阻塞下述 ArtifactStore slice。

## 推荐的最小可闭环切片：TASK-013 + TASK-014

### 目标与范围

把 worker 对 artifact 的直接本地文件操作收敛到 context-aware `ArtifactStore` port，提供 LocalStore（开发/单测）和真实 MinIO/S3-compatible adapter（CI integration）；两个独立 worker 经同一个 bucket 写入/读取同一对象且 content hash/metadata 一致。完成后 production worker 仍须经安全配置/身份等其他门禁；不可仅因有 S3 adapter 就打开 production profile。

### 可能修改的文件

- 新增 `internal/artifacts/store.go`、`local.go`、`s3.go` 和 adapter contract tests；可调整现有 `refs.go`/`refs_test.go`。
- 注入并迁移 `internal/worker/handlers/rag_ingestion_real.go`、`llm_eval.go`、`llm_gateway_eval.go`、`strategy_replay.go`、`side_effect.go` 中的对象读写；`internal/worker/handlers/composite.go` 接收 Store，而非 artifact root 字符串。
- 更新 `cmd/worker/main.go`、`internal/config/config.go` 与配置测试以显式选择实现、校验 bucket/endpoint/TLS/credentials；默认拒绝未配置或不安全 endpoint。
- 增加 MinIO integration test 配置（例如独立 compose/CI service），调整 `.github/workflows/ci.yml`；更新 `deployments/production/README.md`、`docs/operations.md`、`docs/handler-idempotency.md`。
- 原则上不需要 DB Schema/API 改动；只有确需对象元数据持久化时才提出单独 ADR/migration，不把 S3 URI 冒充 task state transaction。

### API / DB / Worker 影响

- API payload 保持受控 `object_key`/artifact reference，不接收任意路径或任意 URL；任何预签名 URL 需另行安全设计。
- DB 当前 result/effect 记录的是引用/元数据；对象写入与 DB commit 不是原子事务。写成功 DB ack 前崩溃会产生 orphan，需可幂等覆盖/确定 key 和后续安全 GC。
- Worker 的输入输出通过 Store；worker 只持最小 bucket 权限。所有调用传 `context.Context`，超时/取消要中止传输并避免错误确认任务成功。

### 关键不变量

1. Object reference 只在授权 bucket/prefix 内；不允许 path traversal、任意 host、任意 bucket 或 credential override。
2. Put/Get 对同一 key + 同一内容可重试；同 key 不同 hash 冲突必须明确失败，不静默覆盖不一致 artifact。
3. 成功结果中的 artifact URI 仅在对象 Put 和验证完成后写入；DB 记录 hash、size/version/content type（按实际契约）。
4. timeout/断连导致 Put 结果未知时，worker 不声称成功；重试通过 Head/hash 判断或安全重放，不产生错误内容。
5. S3 TLS/认证生产配置不降级；未提供 adapter/config 时 worker 继续 fail-closed。

### 验证要求

- 单元/contract：LocalStore 与 S3 adapter 共用 contract tests；key validation、大小限制、metadata/hash、冲突、not-found、取消、超时、权限拒绝。
- PostgreSQL integration：运行 worker 写 artifact 后确认 task result/effect 只记录已验证 reference；在对象成功/DB ack 中断、retry、重复执行时验证最终一致性和无错误 ack。DB 无需变更时明确“不迁移 Schema”。
- Race/Chaos/Failure：两个并发 worker、同 key 幂等写、Put response loss、MinIO restart/network interruption、worker kill-before-ack、权限/凭据撤销；验证不产生伪成功，metrics/log 有低基数错误类别。
- Evidence：记录 commit、Go/Postgres/MinIO 版本、service config（不含密钥）、测试命令与 logs、对象 hash、双 worker run id；新的 manifest/index entry，不修改旧 baseline。
- 完成标准：LocalStore/S3 contract 全过；两 worker 共享结果；未知写结果安全恢复；CI 真实启动 MinIO；handler 不再直接任意文件读写；生产 worker 的 gate 有明确条件和负向测试；runbook 覆盖凭据轮换和 orphan 处理。
- 回滚/恢复：adapter 使用 feature/config switch；未启用时回到 LocalStore 仅限 development。回滚代码前先停止生产 worker，保留已写对象；不回滚已写用户数据或随意删除 bucket。若需要 DB metadata migration，先兼容旧/新双读和备份，再按 expand/contract 回滚。

## 后续建议施工包

每个包在其依赖完成后独立立项，不应一次性施工所有任务。

### TASK-006：OIDC / Workload Identity Verification（P0，产品决策后）

- 目标：验证 issuer、audience、签名、有效期、JWKS key rotation，形成 request-scoped principal；开发静态 token 只允许显式 development profile。
- 文件候选：新增 `internal/auth/` verifier/principal/middleware；修改 `internal/api/router.go`、`internal/config/config.go`、`cmd/api/main.go`；新增 `internal/auth/*_test.go`、router/config tests、CI security tests。
- API/DB/Worker：API context 带 principal；短期 task schema 不必变，service identity 若需持久追溯须 ADR 决定是否存 actor；worker 不直接信任 payload 中 actor 字段。
- 不变量：任何未验签/错误 issuer/aud/过期 token 都 fail closed；客户端不能自赋 role/tenant；密钥轮转期间仅接受受信 JWKS。
- 测试/Evidence：单测覆盖 bad sig/expiry/issuer/aud/key rotation/clock skew；API 集成检查 unauthenticated 与 spoof claims；race 下并发 JWKS refresh；staging 使用测试 IdP 的 E2E 与脱敏 trace evidence。
- 完成/回滚：所有 API 路由明确公开/认证策略；无需真实生产密钥。回滚仅回到 development 配置，production 必须保持拒绝，禁止退回共享 token。

### TASK-007 + TASK-008：授权、IDOR 与管理审计（P0）

- 目标：为 principal 定义资源 scope/RBAC；将审计事件与 privileged mutation 原子化写入，支持按 actor/request/resource 查询。
- 文件候选：`internal/api/router.go`、`internal/api/handlers/task_handler.go`、`internal/application/task_service.go`、`internal/storage/postgres/task_repository.go`；migration 新增独立 audit 表；对应 API/Postgres integration tests 和 `docs/security.md`/ops runbook。
- API/DB/Worker：list/get/events/DLQ/requeue/cancel/queue mutation 全部传 actor scope；task ownership/tenant 列或关联关系需决策，Schema 变更前写 ADR。管理审计字段含 principal ID、request ID、action、resource、outcome、reason、timestamp，不存 token/敏感 payload。
- 不变量：授权在每个资源读取/变更执行；ID 不作为授权；跨 tenant 无侧信道泄露；管理变更与审计记录要么一起 commit，要么二者都不生效；审计不能被普通请求改写。
- 测试/Evidence：单元 policy matrix；Postgres 两租户 IDOR/list/events/DLQ/queue isolation；并发 mutation/race；Failure Lab 在事务提交前后 crash；验证审计 completeness 和 secret redaction。
- 完成/回滚：每类路由有 allow/deny contract，审计完整性可查询；回滚应用需维持 DB 向后兼容，audit table 不级联删除；撤销错误授权策略需紧急 deny-first 开关。

### TASK-010：Cursor / Keyset Pagination（P1）

- 目标：以稳定游标支持大历史分页、筛选 allowlist 和并发写入下无重复/遗漏。
- 文件候选：`internal/domain/task.go`（filter/cursor）、`internal/storage/postgres/task_repository.go`、`internal/api/handlers/task_handler.go`、`api/openapi.json`、API/Postgres integration tests、benchmark scripts。
- API/DB/Worker：增加 opaque signed/versioned cursor，采用 `(created_at,id)` keyset；确认 status/type/queue filter 与排序共同组成 cursor contract；评估复合索引 migration，worker 不受影响。
- 不变量：游标不可篡改/跨筛选复用；排序全序稳定；limit 有上限；不得向任意 SQL 注入字段；删除/状态更新不会导致不稳定排序。
- 测试/Evidence：单测编码/解码/过期和 filter mismatch；Postgres 用 100k representative rows 做 EXPLAIN、正反页、并发 insert/delete、重复/遗漏检测；race API 并发分页；负载基准和 query plan 入 evidence index。
- 完成/回滚：OpenAPI 记录 next cursor/error；所有历史任务筛选覆盖；旧 offset/默认 100 行可设短期兼容，然后按 version policy 弃用。回滚保留旧 endpoint，不删索引/游标数据（游标无持久状态）。

### TASK-015 + TASK-016：可操作 Reconciliation + 一个真实 sandbox provider（P1）

- 目标：将目前 effect simulator 扩展为真实 provider adapter，并让授权 operator 对 unknown outcome 查询/对账，而非 SQL 手工修改。
- 文件候选：`internal/effects/ledger.go`、`internal/storage/postgres/effect_ledger.go`、`internal/worker/handlers/side_effect.go`、新增 provider 包/contract tests、API admin handlers/router/OpenAPI、migration audit columns、`cmd/failurelab`/`cmd/chaosmatrix`、runbook。
- API/DB/Worker：确定 provider idempotency/query semantics；effect intent/attempt/provider request ID/unknown/confirmed/rejected/reconciliation history 明确；管理 API 只写追加处置事件，不能覆盖历史结果。
- 不变量：未知结果不得自动重放非幂等副作用；相同业务 key 不重复产生业务 effect；旧 lease generation 可保留证据但不能覆盖当前 task；人工 resolution 必须由权限和 reason 绑定。
- 测试/Evidence：fake provider contract tests；Postgres ledger concurrency/idempotency；sandbox E2E 使用测试账户；Race/Chaos 在发送后断连、provider 接收后 worker kill、重复 webhook、provider 查询延迟/不可用；保存脱敏 provider request/response IDs 和 reconciliation result。
- 完成/回滚：operator 可看状态并执行安全查询/resolve；provider contract 真正验证。回滚停止新 effect 创建，保留 ledger/history；禁止回滚为“成功/失败”静默重写。

## 发布、安全与韧性后续门

身份/授权、对象存储、provider 和取消语义进入 staging 后，再推进 TASK-019/020/021 可观测闭环、TASK-023 staging E2E、TASK-024 security regression、TASK-027真实依赖故障注入。TASK-018 retention 需 TASK-014 定对象生命周期并先定义审批/导出；TASK-028 需恢复演练；TASK-029/030 需 immutable image 和审批回滚。TASK-040–042 只能在证据链完整后做最终 readiness/resume review。

AI tasks 032–036 是可选轨道：先冻结 scope，再决定真实 vector DB/model/golden set；在 TASK-033/034 未完成前，不把 deterministic hash embedding、strategy simulation、gateway evaluator 宣称为 production RAG 或真实质量基准。

## 本轮停止点

本文件是施工计划，不是实施授权。本轮到此结束并等待用户确认具体 TASK/包、单租户/多租户边界及对象存储目标；未改生产代码、Schema、测试、Tag、历史 evidence，也未运行测试/commit/push。
