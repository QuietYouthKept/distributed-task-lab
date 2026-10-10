# Phase 1 audit: TASK-001–TASK-042 status

审计快照：2026-10-09；仓库 `distributed-task-lab-phase1`，分支 `codex/phase1-production-hardening`，HEAD `916a62e3c4289e0ca4329baee1312ffdc5b936dc`，工作树干净。

## 方法与状态口径

任务名称、原始要求、验收条件及依赖来自只读归档 `C:\Users\13354\Downloads\project_improvement (1).zip` 内的 `project_improvement/17_IMPLEMENTATION_TASKS.md` 和 `18_TASK_DEPENDENCIES.md`。项目原始规划目录未找到；`project_docs` 和 `docs/roadmap.md` 不是 42 项原始定义，故没有用它们代替。归档只读、未解包覆盖。这里按原始规划摘要验收条件，不把文档、接口、Mock 或配置字段单独算作能力。

状态严格按代码、测试、可核验证据判定：DONE=关键验收闭环且有证据；PARTIAL=有实现但有实质缺口/证据不足；TODO=未实现；BLOCKED=当前外部决策或依赖使闭环不能推进。本快照没有标记 BLOCKED：存在需要决策的前置事项，但尚可先做不依赖该决策的工作。

**计数：DONE 5，PARTIAL 20，TODO 17，BLOCKED 0（合计 42）。** 没有新运行测试；下列“Evidence”是仓库已有、可定位的工件，不表示本轮验证了当前 HEAD。

## 任务矩阵

### TASK-001 — Freeze Baseline and Create Evidence Manifest — DONE
- 原始要求/验收：冻结可复现基线；任何人可从 manifest 定位 commit、测试和环境；不能手填性能/覆盖率结论。
- 当前证据：`evidence/index.json:1-20` 指向 `baseline-20261008T141722988Z-f8df9215e0dc`、`measured_valid`、clean tree、scenario `unit+integration+race+failurelab_smoke`；`evidence/README.md` 描述不可覆盖的采集规则。
- 缺口/风险：此 evidence 只验证 `f8df921`，不是当前 `916a62e`；必须重新采集新 HEAD 才能把当前版本称为已验证。
- 测试/Evidence：现有 baseline manifest 和 raw logs；本轮未重跑。
- 依赖/优先级/下一步：无前置，P0；在任何下一阶段验收或发布前为新 commit 采集新 baseline，保留旧 run。

### TASK-002 — Add Typed Domain Errors and HTTP Mapping — DONE
- 原始验收：领域错误不能落成通用 500；错误码映射有契约测试。
- 证据：`internal/domain/errors.go`、`internal/application/errors.go`、`internal/api/contract/errors.go`；`internal/api/contract/errors_test.go` 中 `TestClassifyErrorMappings`、`TestWriteErrorDoesNotLeakCauseAndCarriesRequestID`。
- 缺口/风险：外部客户端兼容性仍需随 API v1 的 OpenAPI 完整化验证；本身的分类与安全响应有测试，不等于所有端点均已覆盖。
- Evidence：baseline commit `f8df921` 的 test manifest；非本轮新验证。
- 依赖/优先级/下一步：001→002，P0；扩展 031 时把全部稳定错误码纳入契约。

### TASK-003 — Validate Task Payloads Before Enqueue — PARTIAL
- 原始验收：每种生产启用 task type 有 schema/测试，不能意外落到 mock handler。
- 证据：`internal/domain/taskpayload/registry.go`、`registry_test.go`、`examples_test.go` 有类型注册、严格字段校验和示例测试；`internal/worker/handlers/composite.go` 按 execution mode 分发；`internal/api/handlers/task_handler.go` 在入队前走应用服务。
- 缺口/风险：worker 的默认 deterministic/mock 路径仍存在；生产 worker 当前 fail-closed，尚无真正生产启用类型清单及端到端“无 mock fallback”证据。Schema 注册不等于可生产执行。
- 测试：`TestValidateProductionTaskPayloads`、`TestValidateRejectsMismatchedAndUnsafePayloads`、`TestCheckedInPayloadExamplesMatchRegistry`、`FuzzValidateRAGData`（仅发现 fuzz target，未本轮运行）。
- 依赖/优先级/下一步：001→003，P0；先定义生产允许的 handler/mode 白名单，对未知或不允许的组合显式拒绝，再以 Postgres/API/worker 集成验证。

### TASK-004 — Replace Production `source_path` with Controlled Object Reference — DONE
- 原始验收：生产 API 不得驱动任意本机文件读取；旧路径行为禁用/弃用。
- 证据：`internal/artifacts/refs.go` 仅接受受控相对 object key，并拒绝路径遍历、绝对路径、分隔符绕过和符号链接逃逸；`internal/worker/handlers/rag_ingestion_real.go` 通过受控根目录解析；`internal/domain/taskpayload/registry.go` real mode 要求 `object_key`；`internal/worker/handlers/rag_ingestion_real_test.go` 有正反测试。
- 缺口/风险：本地 root 仍是单机实现，不是多 worker 共享对象存储；这是 TASK-013/014，不把它重复计入本任务未完成。
- 依赖/优先级/下一步：001→004，P0；在对象存储适配器施工时复用 object-key 信任边界并补跨 worker 测试。

### TASK-005 — Separate Production Configuration and Secret/TLS Guardrails — DONE
- 原始验收：错误生产配置不能启动；仓库不含真实密钥。
- 证据：`internal/config/config.go:143-210` 校验 profile、不同 API/admin token、TLS、非本地 DB、密码、共享 artifact URI、Eval Gateway；`config_test.go` 覆盖严格生产配置和拒绝用例；`cmd/worker/main.go` 调用 `ValidateWorker`。
- 缺口/风险：配置保护只证明 fail-closed 边界，不提供身份系统、密钥轮换、TLS 部署或共享存储。Production worker 明确拒绝启动是安全措施，不是“生产部署已支持”。
- Evidence：CI security workflow 有 secret scan；本轮未触发 CI。
- 依赖/优先级/下一步：001→005，P0；在 TASK-006/014/029 前继续保持拒绝不安全降级。

### TASK-006 — Add OIDC / Workload Identity Verification — TODO
- 原始验收：生产多用户模式不依赖共享静态 token。
- 证据：目前 `internal/config/config.go` 仅读取 `API_TOKEN`/`ADMIN_TOKEN`；`internal/api/router.go` 是 bearer token 等值比较；未发现 issuer/JWKS/claims/principal 实现。
- 缺口/风险：凭据共享、用户撤销/审计和身份归属缺失。必须先决定人类用户、服务账号及可信 issuer 的信任边界。
- 测试/Evidence：无身份验证端到端测试或身份服务 evidence。
- 依赖/优先级/下一步：005→006→007→008；P0（公开/多用户部署前置）；先决策 IdP/issuer、audience、claim→principal 映射及本地开发模式，然后引入 token 验证、过期/签名/JWKS 轮转测试。

### TASK-007 — Implement Authorization Policy — PARTIAL
- 原始验收：普通 principal 无法执行管理变更；隔离声明与 DB 测试相符。
- 证据：`internal/api/router.go` 以 `ADMIN_TOKEN` 守护管理 mutation；`internal/api/router_test.go:35` 的 `TestAdminTokenBoundary` 验证普通 API token 不可调用管理端点。
- 缺口/风险：无 principal、RBAC policy、资源 owner/tenant scope；目前是两把静态 token 的全局粗粒度边界，不能宣称多租户隔离，Get/List/Events/DLQ 的 IDOR 语义也没有 owner 过滤。
- 依赖/优先级/下一步：006→007，P0；将身份 principal 贯穿 API/application，按资源授权并在 DB 查询加 scope，补跨 principal 的 IDOR/越权测试。

### TASK-008 — Add Security/Administrative Audit Events — TODO
- 原始验收：每个 privileged state change 可追溯到 principal 和 request ID。
- 证据：任务事件记录调度状态变化；`internal/api/handlers/task_handler.go` 记录部分普通日志。未发现独立安全审计模型/不可抵赖 actor 字段。
- 缺口/风险：管理 requeue/cancel/queue pause/config 的操作者无法从持久化审计中还原；task event cascade 删除也不能充当长期审计。
- 测试/Evidence：无 privilege audit 的持久化或完整性测试。
- 依赖/优先级/下一步：006→007→008，P0；设计 actor/action/resource/request_id/outcome/reason 及留存/访问控制，和 mutation 在同事务或 outbox 内原子写入。

### TASK-009 — Introduce API v1, Request ID and Stable Error Envelope — DONE
- 原始验收：公开 API 有明确版本边界。
- 证据：`internal/api/router.go:60-74` 注册 `/v1` routes，同时保留旧 routes；`contract/request_id.go` 与 error envelope 实现；`router_test.go` 覆盖 v1 strict contract 和 Request ID。
- 缺口/风险：旧 unversioned routes 并存导致双边界维护；需要明确弃用周期。OpenAPI 漂移仍属于 TASK-031。
- 测试：`TestV1RoutesUseSameStrictContract`、`TestRequestIDIsGeneratedAndInvalidCallerIDIsReplaced`、`TestOpenAPIContainsEveryStableRoute`。
- 依赖/优先级/下一步：002+003→009，P1；在 031 完整化并制定旧路由弃用策略。

### TASK-010 — Add Cursor Pagination and Safe Filtering — PARTIAL
- 原始验收：代表性 100k rows 可分页并有 query plan 数据。
- 证据：`internal/api/handlers/task_handler.go:80-130` 默认 limit=100，status/type/queue allowlist；`internal/storage/postgres/task_repository.go` 支持 bounded limit/offset；`router_test.go` 校验无效筛选。
- 缺口/风险：无 cursor/keyset 稳定分页和 100k 行实测计划；offset 深页会放大扫描/排序成本并在并发写入时产生重复/遗漏。
- 测试/Evidence：仅过滤测试；无 cursor 并发一致性或 100k EXPLAIN bundle。
- 依赖/优先级/下一步：009→010，P1；以 `(created_at,id)` 复合游标实现 next/prev 语义，配合索引/EXPLAIN 与并发插入集成测试。

### TASK-011 — Add API Rate Limiting at the Right Layer — PARTIAL
- 原始验收：单个调用方过载不应轻易拖垮 API 进程。
- 证据：Postgres Queue Rate Limit（`internal/storage/postgres/task_repository.go`；`TestGlobalQueueRateLimitAcrossWorkersAndStableRefresh`）控制 worker 消费速率；`TestQueueMaxConcurrencyConcurrent` 证明队列并发限制。
- 缺口/风险：未发现 API ingress 按 principal/IP 的限流、并发上限或 admission control；消费 rate limit 不能防止 API 入队/昂贵查询滥用。
- 依赖/优先级/下一步：007→011，P1；认证后按主体配置 token bucket/429/Retry-After，保护 DB pool；压测热点主体与公平性。

### TASK-012 — Redesign Public Go Client — PARTIAL
- 原始验收：外部 Go module 能导入并使用 client，不因 internal 可见性失败。
- 证据：`pkg/client/client.go` 为公开包，`go.mod` 提供 module path。
- 缺口/风险：暂无独立外部 module 消费者/兼容性测试；客户端当前端点/分页/错误语义是否覆盖完整 API 未闭环。
- 测试/Evidence：仓库内无 external consumer contract test。
- 依赖/优先级/下一步：009→012，P2；在临时 module fixture 中 `go get`/编译/调用，定义版本兼容和错误类型。

### TASK-013 — Define ArtifactStore Port — TODO
- 原始验收：handler 不再直接调用任意 filesystem path。
- 证据：`internal/worker/handlers` 中多个 handler 仍用 `os.ReadFile`/`os.WriteFile`；目前只有 `internal/artifacts/refs.go` 做本地 key 安全解析，不是 Store 接口。
- 缺口/风险：handler 与本地磁盘耦合；多 worker 无共享语义、对象生命周期和校验契约。
- 测试/Evidence：没有 Store contract/conformance tests。
- 依赖/优先级/下一步：004→013，P1；定义 context-aware Put/Get/Head/Delete、对象引用、校验和及错误语义，先迁移 LocalStore。

### TASK-014 — Implement S3/MinIO-compatible Artifact Store — TODO
- 原始验收：多 worker 可观察到一致 artifact 语义。
- 证据：`deployments/production/README.md` 明示尚无 MinIO/S3 adapter；production worker fail-closed；Compose 使用本地 volume。
- 缺口/风险：跨主机 worker 不能共享输出；local path 误用风险由 fail-closed 避免，但真实生产执行尚不可用。
- 测试/Evidence：无 MinIO 服务、兼容性/故障测试或共享存储 evidence。
- 依赖/优先级/下一步：013→014→018/023，P1；选定 MinIO/S3 兼容语义并以真实服务做 adapter contract 和双 worker 可见性/重试验证。

### TASK-015 — Build Effects Query/Reconciliation/Admin Workflow — PARTIAL
- 原始验收：操作员无需直接 SQL 即可解决未知副作用结果。
- 证据：`internal/effects/ledger.go`、`internal/storage/postgres/effect_ledger.go` 和 migration 009 有 intent/receipt ledger；`effect_ledger_integration_test.go` 验证幂等/旧 generation 证据保留；handler tests 覆盖模拟 response loss。
- 缺口/风险：没有查询、人工 resolve/reconcile API/UI/CLI；ledger 存在不代表 on-call 能闭环。实际 provider 尚未接入。
- 依赖/优先级/下一步：007→015→016/023/027，P1；做受授权管理工作流、状态冲突检查、审计和只追加的 reconciliation 记录。

### TASK-016 — Integrate One Real Sandbox Side-effect Provider — TODO
- 原始验收：provider effect end-to-end 有证据。
- 证据：`internal/worker/handlers/side_effect.go` 是确定性本地 simulator/receipt；failurelab/chaosmatrix 中 provider loss 场景亦是模拟。
- 缺口/风险：无沙箱 API、真实 provider idempotency key、查询状态和 reconciliation；“exactly once”不可宣称。
- 测试/Evidence：本地模拟测试不是 provider evidence；无外部 sandbox 合约测试。
- 依赖/优先级/下一步：015→016→023/027，P1；选择可重复沙箱 provider，先完成 request idempotency、timeout unknown、查询恢复，记录脱敏原始响应和 provider request id。

### TASK-017 — Propagate Cancellation to Running Handlers — PARTIAL
- 原始验收：取消确定、文档明确，失去 ownership/state 后不能成功 ack。
- 证据：`internal/worker/pool.go` 及 `pool_test.go` 有 context cancel、ownership loss 取消和不 ack；`TestPoolOwnershipLossCancelsAndDoesNotAck`、`TestPoolGracefulShutdown`。
- 缺口/风险：API Cancel 更新 DB fencing state，但无可靠主动通知到当前 worker 的信道；handler 可能直到轮询/lease renew 才观察；跨外部副作用取消语义仍未知。
- 依赖/优先级/下一步：003→017→023/027，P1；明确取消线性化点、传播延迟边界、handler合作取消及 side-effect unknown 状态。

### TASK-018 — Implement Retention and Safe Cleanup — TODO
- 原始验收：操作员能预测删除内容并证明不触碰 live task。
- 证据：`docs/storage-lifecycle.md` 仅有建议流程；无 retention worker/命令或 dry-run。
- 缺口/风险：表无限增长；手工删除可能破坏审计、效果关联或运行任务。
- 测试/Evidence：无保留策略、dry run、批量清理集成测试。
- 依赖/优先级/下一步：014→018→028，P2；先定义各数据分类保留期限、导出校验及只允许 terminal 状态的分批删除。

### TASK-019 — Standardize Structured Logging and Metrics — PARTIAL
- 原始验收：事件可关联 request→task→attempt→provider，且不泄漏 secret。
- 证据：`internal/observability/logger.go` 用 JSON slog；`metrics.go`、`metrics_test.go` 存在；API 传播 Request ID/trace context。
- 缺口/风险：跨 provider 的统一字段/脱敏规则与日志泄密负向测试未证明；metrics label cardinality/预算及 task attempt/provider 关联不完整。
- 依赖/优先级/下一步：002→019→020/021，P1；定义 schema、敏感字段 denylist/redaction、低基数 labels 和日志/指标回归测试。

### TASK-020 — Complete End-to-End OpenTelemetry Tracing — PARTIAL
- 原始验收：staging trace 可见 API→enqueue→worker→external→ack。
- 证据：`internal/api/router.go` 使用 otelhttp；`task_handler.go` 把服务端 trace 注入 task payload；Postgres claim spans 有 trace attributes。
- 缺口/风险：未见 tracer provider/exporter/collector/staging trace evidence；worker→provider→ack 全链路可视化未证。
- 测试/Evidence：目前是代码路径和 baseline 测试，不是可见 staging trace。
- 依赖/优先级/下一步：019→020→021，P1；配置 OTLP exporter、采样/敏感数据策略，采集端到端 trace 并验证断链/错误 span。

### TASK-021 — Define SLIs, Dashboards and Alerts — TODO
- 原始验收：on-call 能从遥测判断故障及开始版本。
- 证据：Prometheus/Grafana compose 和 overview dashboard 存在；未发现 alert rules、SLO/SLI 定义、release 标记。
- 缺口/风险：仪表盘不能主动告警；无告警路由、误报/漏报演练。
- 依赖/优先级/下一步：019→020→021→030，P1；定义 queue age、claim/complete/error、DB/pool、provider unknown 的 SLI，做版本标签和可运行告警演练。

### TASK-022 — Formalize Test Data, Coverage Reporting and Critical-package Gates — PARTIAL
- 原始验收：回归/perf 结果注明 dataset version 和 generator config。
- 证据：`.github/workflows/ci.yml` 执行 go test 并上传 coverage.out；payload examples、eval receipts 有版本/哈希字段。
- 缺口/风险：无 coverage gate/阈值、关键包变化门禁和统一测试数据生成器版本策略；上传 raw coverage 不等于 coverage gate。
- 依赖/优先级/下一步：003→022，P1；定义数据集 manifest/fixture generator version，关键包变更门禁，发布 coverage 摘要并保持不伪造指标。

### TASK-023 — Add Production-like Staging E2E Suite — TODO
- 原始验收：关键路径失败不得 promotion。
- 证据：目前 CI 的 Postgres integration/race；未见 staging 环境、身份/对象存储/provider E2E 或 promotion gate。
- 缺口/风险：组件单测不涵盖真实部署拓扑、凭据、TLS、共享存储及恢复。
- 依赖/优先级/下一步：007+014+016+017→023，P1；在非生产 staging 验证 API→DB→worker→object store→provider，promotion fail-closed。

### TASK-024 — Build Security Regression Suite — PARTIAL
- 原始验收：P0 trust-boundary 问题有永久回归测试。
- 证据：`.github/workflows/security.yml` 有 govulncheck/CodeQL/secret scan；`internal/artifacts/refs_test.go`、payload/API strict parsing 测试有局部信任边界保护。
- 缺口/风险：无统一 security regression suite；OIDC/RBAC/IDOR/SSRF/provider credential 保护缺少测试；静态扫描不替代授权测试。
- 依赖/优先级/下一步：004+007→024，P0；按 trust-boundary threat model 将负向用例加入 CI 并固定防回归。

### TASK-025 — Create Database Query and Lock Benchmark Matrix — PARTIAL
- 原始验收：索引/refactor 建议均有实测查询或计划依据。
- 证据：`internal/storage/postgres/benchmark_test.go`、`benchmarks/README.md`、多个 worker scaling bundles 存在。
- 缺口/风险：已有 worker 吞吐 bundle 不等于 Postgres query/lock contention 的 query-plan matrix；未看到各关键查询 EXPLAIN/锁等待基准的 indexed evidence。
- 依赖/优先级/下一步：001→025，P1；为 claim/list/renew/DLQ/retention 查询保存代表性数据规模、EXPLAIN ANALYZE、锁等待和原始结果。

### TASK-026 — Run API/Worker Load, Stress, Spike and Soak Benchmarks — PARTIAL
- 原始验收：可重复 baseline/scaling matrix 有 raw evidence，识别瓶颈。
- 证据：`cmd/loadgen`、`benchmarks/workers-{1,4,8,16}-queues-4-payload-1k/` 中 manifest/raw CSV/summary/report/Postgres stats。
- 缺口/风险：工件不在 `evidence/index.json` 的 measured_valid 记录中；本轮未核查每 bundle 环境/commit 与当前可复现性，也未见 API spike/长时 soak 证据。不可引用未经核验的吞吐/延迟数值。
- 依赖/优先级/下一步：019+025→026，P1；重跑矩阵、记录资源限制与 commit，生成可追溯证据并明确瓶颈/置信区间。

### TASK-027 — Automate Failure Injection Matrix — PARTIAL
- 原始验收：worker/provider/object store/DB 故障能得到预期 state 与 observability。
- 证据：`cmd/failurelab`、`cmd/chaosmatrix`（列出 worker kill、drain、stale generation、effect-before-ack、provider response loss、Postgres disconnect）；baseline scenario 包含 failurelab smoke。
- 缺口/风险：场景目前是内存/模拟 provider；没有真实对象存储/provider 故障或 staging telemetry 验证，12/12 历史场景不能外推为生产 chaos 覆盖。
- 依赖/优先级/下一步：014+016+017→027，P1；将故障注入扩展到可控 staging，并校验状态、告警、重试/恢复及清理。

### TASK-028 — Implement Backup/Restore and Disaster Recovery Drill — TODO
- 原始验收：备份必须真实恢复并验证。
- 证据：迁移/运维文档建议备份；未发现自动备份策略、恢复脚本或 drill evidence。
- 缺口/风险：数据库/object artifacts 恢复点不一致、RPO/RTO 未定义，发生故障无法证明可恢复。
- 依赖/优先级/下一步：018→028，P1；定义 RPO/RTO 与 PostgreSQL/object snapshot 一致性，定期隔离环境恢复并校验任务、events、effects、artifact hashes。

### TASK-029 — Produce Immutable Release Images with Metadata and Scanning — TODO
- 原始验收：运行中的 binary/image 可追溯到 commit 且扫描。
- 证据：有 Dockerfiles 和 security workflow；无 immutable digest release 流程/签名/SBOM/扫描证明。
- 缺口/风险：latest/tag 可变、镜像来源和部署版本 provenance 不闭环。
- 依赖/优先级/下一步：005→029→030，P1；pin base image，构建一次晋级、附 commit/SBOM/scan/signature，拒绝可变产物。

### TASK-030 — Add Staging Deployment, Approval, Production Promotion and Rollback — TODO
- 原始验收：部署/回滚重复可行并有 evidence。
- 证据：只有 development Compose 和 production boundary README；没有 staging/prod deployment workflow、approval、rollback 演练。
- 缺口/风险：迁移兼容性、secret 注入、配置漂移和回滚步骤未实操。
- 依赖/优先级/下一步：005+029+021+023→030，P1；以不可变 artifact 部署 staging，经人工批准 promotion；验证回滚及 schema expand/contract。

### TASK-031 — Generate and Enforce OpenAPI Contract — PARTIAL
- 原始验收：所有公开 endpoint/schema/error 进入 contract，CI 检测 drift。
- 证据：`api/openapi.json`；`internal/api/openapi_contract_test.go` 检查稳定 route 和 Create contract。
- 缺口/风险：当前测试只覆盖选定 routes/Create contract；无生成器或全量 schema/error parity/drift lint 门禁。
- 依赖/优先级/下一步：009→031，P1；补齐每个 v1 path、鉴权、错误、分页、examples，并在 CI 比对生成物。

### TASK-032 — Freeze AI Scope and Add Prompt/Model Provenance — PARTIAL
- 原始验收：README/resume 不把 deterministic simulation 说成模型质量。
- 证据：task mode 区分 mock/deterministic/gateway；`internal/worker/handlers/llm_gateway_eval.go` receipts 记录 dataset/model/prompt/retriever/schema versions；docs 有 AI eval 说明。
- 缺口/风险：未找到经过批准冻结的 AI scope/claim ledger；`RealRAGIngestion` 的 embedding 为 deterministic SHA256，不是真模型；历史结果不能证明可复现模型评估。
- 依赖/优先级/下一步：001→032→033…，P2/AI optional；冻结可说/不可说声明、来源字段及证据要求，公开标明 deterministic 与真实 provider 边界。

### TASK-033 — Implement a Real RAG Retrieval Vertical Slice (Optional AI Track) — TODO
- 原始验收：top-k 必须来自真实 vector retrieval，而不是 deterministic hash。
- 证据：`rag_ingestion_real.go` 实际切分文档并产出确定性 SHA256 特征；没有 vector index/retriever/generation 路径。
- 缺口/风险：将当前 ingest/replay 标为 RAG retrieval 会夸大能力；无真实召回/生成质量 evidence。
- 依赖/优先级/下一步：032→033→034/035，P2 optional；只有数据/模型/向量基础设施决策后再做真正 top-k vertical slice。

### TASK-034 — Build and Double-review a Frozen RAG Evaluation Dataset (Optional AI Track) — TODO
- 原始验收：每条冻结测试都有双人审阅的 expected evidence/answer。
- 证据：有负载 examples、eval manifest 字段，但未发现冻结且双审的黄金数据集与审阅记录。
- 缺口/风险：测试集泄漏、期望答案漂移、指标无法比较。
- 依赖/优先级/下一步：032→034→035，P2 optional；固定版本/hash、来源授权、标注 rubric、双人审批及变更流程。

### TASK-035 — Implement Real Retrieval/Generation Evaluation Runner (Optional AI Track) — PARTIAL
- 原始验收：指标必须追溯到冻结 dataset 和 raw retrieval/prediction。
- 证据：`llm_gateway_eval.go` 有 HTTP evaluator gateway、case receipts、dataset hash/version/prompt/model 字段；`llm_gateway_eval_test.go` 测试 artifact resume。
- 缺口/风险：评估 runner 不等于 RAG 检索/生成执行；当前未见真实 retrieval output + frozen gold set + raw predictions 的端到端闭环，且 TASK-034 未完成。
- 依赖/优先级/下一步：032→033+034→035，P2 optional；先补输入/输出 provenance，再形成可独立复算的 raw records 与指标。

### TASK-036 — Add AI Security and Regression Gate (Optional AI Track) — TODO
- 原始验收：高风险 AI regressions 单独可见且不被均值掩盖。
- 证据：没有专门 prompt injection/data exfiltration/tenant leakage/quality regression gate。
- 缺口/风险：常规 Go security CI 不覆盖 retrieval poisoning、敏感数据泄露、提示攻击和评估分层。
- 依赖/优先级/下一步：032→033→034/035→036，P2 optional；在有真实 AI 路径之后设计攻击集/严重度 gate，原始 case 可追溯。

### TASK-037 — Remove/Marginalize Misleading Redis Skeleton and Historical Noise — PARTIAL
- 原始验收：reviewer 不会误把未实现 Redis/历史分支当成现有架构。
- 证据：`internal/storage/redis/redis_queue.go` 存在 `NotImplemented` skeleton；当前架构文档主要指向 Postgres。
- 缺口/风险：源码中保留看似适配器的空实现，容易误判可替换性；需确认README/架构清楚标成未实现以及排除陈旧快照。
- 测试/Evidence：无运行测试需要；属可读性/架构声明验收。
- 依赖/优先级/下一步：001→037，P2；移到明确 `experimental`/删除前先审计引用，补架构说明并确保无生产构建依赖。

### TASK-038 — Decompose PostgreSQL Repository by Semantic Concern — TODO
- 原始验收：可读性提升，不做架构重写。
- 证据：`internal/storage/postgres/task_repository.go` 聚合任务、队列 governance、stats、events、worker heartbeat、lease、DLQ 等多种职责。
- 缺口/风险：高变更冲突、SQL 所有权不清，难独立评审 lock invariant。
- 测试/Evidence：已有 Postgres integration 可作行为基线；无 refactor evidence。
- 依赖/优先级/下一步：001→038，P2（在基准之后）；按语义拆文件不改事务/SQL/接口行为，逐步保持测试结果。

### TASK-039 — Add ADRs and Operational Runbooks — PARTIAL
- 原始验收：设计原因和 incident action 不依赖口口相传。
- 证据：`docs/operations.md`、`docs/failure-model.md`、`docs/migrations.md`、`docs/handler-idempotency.md` 等有操作/设计资料。
- 缺口/风险：未发现 ADR 序列；runbook 尚未覆盖真实身份服务、MinIO/provider unknown、备份恢复和生产告警。
- 依赖/优先级/下一步：001→039，P1；针对选定实现补 ADR 与有责任人/验证步骤的 incident runbook。

### TASK-040 — Automate Evidence Index and Resume Metric Provenance — PARTIAL
- 原始验收：每个简历数字都能映射到 measured evidence。
- 证据：`evidence/README.md` 和 `scripts/evidence/collect-baseline.ps1` 设计为自动生成 immutable manifest/index；index 记录 baseline `f8df921`。
- 缺口/风险：当前 HEAD `916a62e` 的 index 仍只记录旧 commit；worker benchmark bundles 尚未成为 measured_valid record。不能把历史 baseline 说成当前工作树实测。
- 依赖/优先级/下一步：026+027+028+030+035→040→041/042，P1；每次 claim 绑定 commit/evidence ID/artifact，采集器校验链接与 commit 一致。

### TASK-041 — Re-run Production Readiness Assessment — TODO
- 原始验收：任何 readiness 维度不能只靠意图/设计文档支撑。
- 证据：本文件是任务审计快照，不是生产 readiness assessment；production worker 当前 fail-closed。
- 缺口/风险：身份、共享存储、真实 provider、DR、staging、alerts 均未闭环。
- 依赖/优先级/下一步：026+027+028+030+035→040→041，P1；依原 42 项验收矩阵用新 evidence 做实测复评。

### TASK-042 — Final Resume and Interview Evidence Review — TODO
- 原始验收：简历数字零无 evidence ID；不宣称 exactly-once、production RAG、multitenant/Kubernetes 而无代码证明。
- 证据：Phase 0 baseline evidence 存在；Phase 1 最终审阅尚未发生。
- 缺口/风险：历史演示/模拟场景容易被误述为真实生产能力。
- 依赖/优先级/下一步：040→042，P2/收尾；逐条核验简历、demo 与 evidence，删改超出证明范围的表述。

## 跨任务说明

**Phase 0 原始验收复核：** TASK-001、002、004、005 的对应边界具备实现和测试/Evidence；TASK-003 仍 PARTIAL，因此不能说 Phase 0 的 TASK-001～005 全部满足原始验收条件。当前 HEAD 的历史证据不覆盖 `916a62e`。`TASK-009`（v1 routes/request ID/error envelope）以及一部分 queue governance、payload、安全路径和 observability 在阶段规划之外已提前出现，但不能据此把后续任务整体判 DONE。

`a929318` 不在当前 HEAD 祖先链；`Makefile` 与 `evidence/README.md` 的工作树内容和该提交的变更完全一致，由当前 `916a62e` 带入。因此记录为“补丁内容已包含，提交对象未 cherry-pick”，不改写提交历史。
