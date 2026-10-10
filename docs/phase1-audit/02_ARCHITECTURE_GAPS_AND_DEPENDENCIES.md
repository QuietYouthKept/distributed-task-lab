# Phase 1 architecture gaps and dependencies

审计快照：2026-10-09，Phase 1 HEAD `916a62e3c4289e0ca4329baee1312ffdc5b936dc`。结论基于当前工作树及已有工件；本轮未运行测试或连接生产基础设施。

## 实际架构（非目标架构）

```text
HTTP API (Go net/http, /v1 + legacy routes, static API/Admin bearer tokens)
  -> application/domain payload validation
  -> PostgreSQL queue (migrations, SKIP LOCKED, leases/fencing, retry/DLQ,
     queue pause/concurrency/rate/aging, task events, effect ledger)
  -> worker pool (lease renewal, backpressure, context cancellation)
  -> explicit handler modes
       mock/deterministic simulation
       local filesystem RAG ingestion / strategy replay / eval artifacts
       HTTP eval gateway
       local simulated payment/notification side-effect + ledger
  -> local disk artifacts (production worker deliberately refuses startup)
```

PostgreSQL 是当前持久队列和并发控制的真实实现。Worker 通过 PostgreSQL 行锁与 `SKIP LOCKED` 认领；租约与 retry 用 `clock_timestamp()`；`lease_version` 是 fencing token。队列级 pause、最大并发、rate token bucket、priority aging 有迁移和集成测试。数据库迁移 001–009 包含任务、事件、worker、幂等/fencing/retry、queue governance/rate limit/integrity/aging/effect 表。

API 已有 `/v1` 路由、request ID、错误 envelope、严格 JSON decode、`Idempotency-Key` 和 OpenAPI 文件。但认证是两种静态 bearer token，不是用户/服务 principal；授权是普通 API 与全局管理 token 的粗粒度切分。任务 API 没有 cursor pagination。

### 生产安全边界与演示边界

`internal/config/config.go` 校验生产 profile、API/admin token 不同、DB TLS/host/password、共享 artifact URI、HTTPS Eval Gateway 等；worker 在 production profile 明确拒绝启动，因为 handler 仍用本地 filesystem 且无 S3/MinIO adapter。这是有意的 fail-closed：它避免把 `s3://` 当作本地路径，不能被描述为“生产 worker 已部署”。`deployments/production/README.md` 明确说明该限制。

“real RAG ingestion”是真实文件读取、切分和 artifact 写入，但 embedding 是 deterministic SHA256 特征，没有 vector store/top-k/retrieval/generation。Strategy replay 生成确定性报告。Eval gateway 可向外部 evaluator 发 HTTP 请求并写 receipts，但没有冻结双审数据集和真实 retrieval/generation 闭环。Payment/notification effect path 有 intent/receipt ledger，但执行方是本地 simulator，不是真实 provider。

## Phase 0 独立复核

- Phase 0 实现 commit：`f8df921`；verified evidence commit/tag：`64fd7bb` / `upgrade-p0-complete-20261008`。Tag 未移动。
- `evidence/index.json` 唯一 `measured_valid` 项的 `commit_sha` 为 `f8df9215e0dc55f7f07580c1672edd99b9c4dcc0`，采集时间 2026-10-08；scenario 为 unit+integration+race+failurelab smoke。它不是当前 HEAD `916a62e` 的验证结果。不得复用成新版本 evidence。
- TASK-001、002、004、005 有对应实现和测试/安全 evidence；TASK-003 的 payload schema 有覆盖，但“production-enabled task 类型 + 无意外 mock fallback”的生产执行验收仍不闭环。因此 Phase 0 的 TASK-001～005 不能整体判为全部满足。
- TASK-009 v1/request ID/error envelope 已提前实现；队列治理、API 严格解析、有限的 trace/metrics、effect ledger 等也先于原路线部分出现。
- `a929318` 不在当前提交祖先链；其 `Makefile` 与 `evidence/README.md` patch 与 HEAD 内容相同，由 `916a62e` 实现。准确状态是变更已包含、原提交未 cherry-pick。

## 最高优先级差距

### P0 — 信任边界和证据可信度

1. **身份与资源授权缺失（TASK-006/007）**：共享静态 bearer token 无法可靠识别个人/服务主体；没有 tenant/owner scope。猜测 task ID 访问 Get/List/Events/DLQ 的 IDOR 风险未由 principal 约束。
2. **管理操作不可归责（TASK-008）**：pause、queue config、requeue、cancel 等 privileged mutation 没有持久化 principal/request ID 审计事件。
3. **payload 到 handler 的生产准入不闭环（TASK-003）**：有 schema 与显式 mode，但 mock/deterministic 实现仍存在，未形成生产 allowlist 和端到端拒绝保证。
4. **Evidence 时效边界（TASK-001/040）**：唯一 `measured_valid` 指向 f8df921，而当前分支已前进到 916a62e。若把旧 evidence 作为当前 HEAD 结果展示，会误报验证范围。
5. **安全回归尚未覆盖身份/租户/SSRF/外部副作用**（TASK-024）：静态扫描有帮助，但不能证明应用层授权和数据隔离。

### P1 — 生产关键执行链路

6. **多 worker artifact 共享缺失（TASK-013/014）**：worker 使用本地目录；Compose volume 仅是单 Docker 主机共享模型，不能证明多主机一致语义；production worker 因此 fail-closed。
7. **外部 effect 未真实执行/操作闭环缺失（TASK-015/016）**：ledger 是真实 PostgreSQL 持久数据结构，但 provider 是 simulator；无 operator query/reconcile/admin API，unknown outcome 不能由真实 provider 查询恢复。
8. **API 大历史分页和入口保护缺失（TASK-010/011）**：只有 limit/offset/bounded filter；无 cursor 100k 验证；worker queue rate limit 不保护 API ingress。
9. **Staging、发布、回滚和 DR 证据缺失（TASK-023/028/029/030）**：development Compose 不能替代 staging；没有 immutable image promotion、实际 restore drill、RPO/RTO。
10. **遥测链路不闭环（TASK-019/020/021）**：结构化日志、基础 metrics/tracing 代码和 dashboard 存在，但未有 OTLP exporter/staging full trace、SLI/告警规则/演练及版本标识。

其他重要缺口：TASK-017 的 API cancellation 未主动传播到运行中的 worker；TASK-018 无 retention worker；TASK-025/026 bundle 尚未进入有效 evidence provenance；TASK-031 OpenAPI drift 检查有限；TASK-038 仓库语义过于集中；AI optional track 无真实 retrieval + 双审数据集。

## 依赖与施工顺序

原始 dependency graph（来自原归档 `18_TASK_DEPENDENCIES.md`）：

```text
001 -> 002,003,004,005
005 -> 006 -> 007 -> 008
002+003 -> 009 -> 010,012,031
007 -> 011,015,024,023
004 -> 013 -> 014 -> 018,023
015 -> 016 -> 023,027
003 -> 017 -> 023,027
002 -> 019 -> 020,021; 019+025 -> 026
003 -> 022; 001 -> 025,032,037,038,039
014+016+017+007 -> 023; 004+007 -> 024
014+016+017 -> 027; 018 -> 028
005 -> 029 -> 030 (also 023,021 -> 030)
026+027+028+030+035 -> 040 -> 041 -> 042
032 -> 033 -> 034/035 -> 036
```

建议先定一项产品决策，而不是虚构 TASK ID：系统第一目标是单租户内部任务平台还是多租户/public API？这会决定 TASK-006 issuer/claim、TASK-007 tenant policy、TASK-008 审计主体及 API 限流 key。此决策不阻塞 TASK-013 的对象存储 Port。

推荐最小可闭环施工切片：**TASK-013 + TASK-014（ArtifactStore port + MinIO/S3 兼容真实 adapter）**。理由是其边界可独立验证、原路线 004 已给受控 object key 前置、生产 worker 已明确因该缺口 fail-closed；它能把“多 worker 共享 artifact”从本地模拟推进到真实服务验证。选择 MinIO 作为 CI 可重复 S3-compatible target 是建议，正式云端供应商/认证/保留策略仍需决策。它不会自动解决身份/RBAC/provider/生产部署问题，完成后仍不得宣称 production-ready。

## 需要产品/平台决策

- 用户面：单租户内部控制台还是面向多个租户/外部 API；人员账户、service account、issuer/audience、tenant claim 是什么。
- S3 兼容性目标：MinIO CI target、AWS S3/其他云 provider、路径风格/版本/加密/对象大小和生命周期；凭据使用 workload identity 还是 secret。
- 外部副作用：选择一个 sandbox provider、测试账户/回调能力、provider idempotency/query API、unknown outcome 的业务处置权和人工审批。
- 数据治理：task/payload/event/effect/artifact 的保留期限、个人信息分类、删除法律边界、归档不可变性。
- 运行目标：staging/production 部署平台、on-call/SLO、RPO/RTO、镜像 registry/signing 和审批人。
- AI track：是否继续；若继续，允许的数据集/模型/向量服务、预算、隐私/安全评审与可公开 claim。

## 证据约束

`benchmarks/workers-*` 中有 manifests/raw CSV/reports，但 `evidence/index.json` 的 measured_valid 项只有旧 baseline。bundle 只能称为“现存 benchmark artifacts”，不能在未核查 commit、环境、命令及可复现性的情况下拿来宣称吞吐或延迟。历史 Failure Lab 通过记录同理：它证明该 baseline 上执行过指定模拟场景，不证明真实 provider/object store、staging chaos 或 production SLA。
