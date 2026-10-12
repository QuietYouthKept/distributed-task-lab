# 本地运行资源归属与安全回收方案

更新时间：2026-10-11 12:30 UTC（容器资源采样约 12:30 UTC）

目标工作树：当前 E 盘隔离 Worktree（绝对路径不写入版本库）

用途：在不影响其他项目数据和运行任务的前提下，为 TaskLab 的真实 PostgreSQL 验收确认资源门禁与可恢复的人工回收路径。

## 结论

本次门禁未通过：Windows 可用物理内存 **5.88 GiB**，低于容器测试要求的 **6 GiB**。本快照显示 24 个运行中容器；没有容器带有 TaskLab 本地运行项目标签。其余 Compose 项目均归属其他仓库/实验，另有一个 Redis 容器没有 Compose 所有权标签。**本次未停止、重启、删除或修改任何容器、卷、网络、WSL 发行版或其他项目文件。**

因此不启动 PostgreSQL、MinIO、Compose 集成测试、Race、镜像拉取或其他容器级验证。恢复门禁后，应重新采样，不能把本报告中的旧快照当作恢复证明。

## 主机资源快照

| 指标 | 观测值 | 判断 |
|---|---:|---|
| 物理内存 | 31.22 GiB 总量；5.88 GiB 可用 | 低于容器测试 6 GiB 门槛；低于 4 GiB 才会触发的编译/Race 硬停门槛尚未触发，但不代表可启动容器测试 |
| TIME_WAIT | 462 | 低于 5,000 门槛 |
| E: 可用空间 | 49.47 GiB | 当前工作树位于 E:；空间充足 |
| Docker | Server 28.3.3；Linux VM MemTotal 15.23 GiB；24 CPUs | VM 总量不是 Windows 可用内存，不能与主机 RAM 相加 |
| WSL | Ubuntu-24.04 Stopped；docker-desktop Running | 未启动/关闭任何发行版 |
| 容器 | 24 个运行中 | 全部需要按所属项目确认；没有 TaskLab 容器 |
| vmmemWSL | Working Set 5.65 GiB；Private 15.59 GiB | 进程计数与 Docker VM/缓存有重叠，不可简单求和，也不能推算停止容器后可释放量 |

Windows 当前 Working Set 较高的进程（GiB；只作为归因线索，不是可回收额度）：

| 进程 | Working Set | Private |
|---|---:|---:|
| vmmemWSL | 5.65 | 15.59 |
| Memory Compression | 1.07 | 0.01 |
| ChatGPT | 1.01 | 1.22 |
| chrome（最大实例） | 0.74 | 0.99 |
| com.docker.backend | 0.72 | 1.17 |
| chrome | 0.52 | 0.57 |
| MsMpEng | 0.49 | 0.68 |
| chrome | 0.47 | 0.51 |
| chrome | 0.46 | 0.46 |
| chrome | 0.42 | 0.51 |
| chrome | 0.39 | 0.53 |
| explorer | 0.34 | 0.59 |
| ChatGPT | 0.32 | 0.47 |
| chrome | 0.31 | 0.44 |
| Weixin | 0.29 | 0.46 |

不建议通过调整 Windows 全局网络/内存设置，或关闭 Docker Desktop/WSL 来绕过门禁。

## 运行容器清单

CPU 和内存来自 `docker stats --no-stream` 的瞬时采样；CPU 可超过 100%（多核）。Compose 项目/配置路径来自容器标签。所有 Compose 路径仅用于辨认归属，没有对这些文件执行写入。

| 容器 | 所属/配置 | 镜像 | 启动时间 UTC | CPU | 内存 |
|---|---|---|---|---:|---:|
| `strategyops-v33-validation-api-1` | StrategyOps；`[StrategyOps 仓库]\docker-compose.v33-validation.yml` | `strategyops-finalization:v32-w4-24bc299` | 12:28:50 | 0.12% | 89.66 MiB / 768 MiB |
| `strategyops-v33-validation-worker-1` | 同上 | `strategyops-finalization:v32-w4-24bc299` | 12:28:50 | 0.37% | 81.80 MiB / 768 MiB |
| `strategyops-v33-validation-collector-1` | 同上 | `otel/opentelemetry-collector-contrib:0.135.0` | 12:28:46 | 0.00% | 29.08 MiB / 256 MiB |
| `strategyops-v33-validation-db-1` | 同上 | `postgres:16@sha256:ca0bd484…` | 12:28:33 | 0.86% | 40.18 MiB / 512 MiB |
| `gateway-wave4-capacity-gateway-1` | Gateway；`[Gateway 测试产物]\wave4-capacity-20261011\capacity-compose.yml` | `mini-llm-gateway:wave4-observability-5af8cf5` | 12:02:15 | 183.01% | 79.69 MiB / 1 GiB |
| `gateway-wave4-capacity-redis-1` | 同上 | `redis:7-alpine` | 11:23:58 | 0.30% | 6.34 MiB / 512 MiB |
| `gateway-wave4-capacity-postgres-1` | 同上 | `postgres:16-alpine` | 11:23:58 | 186.14% | 220.60 MiB / 1 GiB |
| `strategyops-v31-rag3-20261011-api-1` | StrategyOps；主配置 `[StrategyOps 仓库]\docker-compose.finalization.yml` + `[StrategyOps 运行产物]\v31-resume-20261011\compose-override-candidate.yml` | `strategyops-finalization:v31-w2-closed-b42d161` | 10:50:50 | 0.11% | 87.89 MiB / 768 MiB |
| `strategyops-v31-rag3-20261011-failure-api-1` | 同上 | `strategyops-finalization:v31-w2-closed-b42d161` | 10:42:28 | 0.12% | 98.91 MiB / 512 MiB |
| `gateway-wave3-capacity-20261011t0602-gateway-1` | Gateway；`[Gateway 仓库]\deploy\wave5\compose.yaml` + `[Gateway 测试产物]\production-hardening-wave3-20261011T055445Z\04-capacity-matrix\compose-capacity-override.yaml` | `sha256:eb587db5…` | 06:08:10 | 25.72% | 82.27 MiB / 1 GiB |
| `gateway-wave3-capacity-20261011t0602-redis-1` | 同上 | `redis:7-alpine` | 10:57:55 | 3.25% | 8.73 MiB / 512 MiB |
| `gateway-wave3-capacity-20261011t0602-postgres-1` | 同上 | `postgres:16-alpine` | 10:54:31 | 0.00% | 40.00 MiB / 1 GiB |
| `strategyops-v31-rag3-20261011-db-1` | StrategyOps；主配置 `[StrategyOps 仓库]\docker-compose.finalization.yml` + `[StrategyOps 运行产物]\v31-20261011\compose-override-retry2.yml` | `postgres:16@sha256:ca0bd484…` | 06:04:04 | 0.00% | 49.20 MiB / 512 MiB |
| `gateway-wave3-diag-20261011t055445-gateway-1` | Gateway；`[Gateway 仓库]\deploy\wave5\compose.yaml` + `[Gateway 测试产物]\production-hardening-wave3-20261011T055445Z\03-historical-503-diagnostic\compose-network-override.yaml` | `sha256:eb587db5…` | 05:56:53 | 17.18% | 67.37 MiB / 15.23 GiB |
| `gateway-wave3-diag-20261011t055445-postgres-1` | 同上 | `postgres:16-alpine` | 05:56:48 | 0.00% | 34.45 MiB / 15.23 GiB |
| `gateway-wave3-diag-20261011t055445-redis-1` | 同上 | `redis:7-alpine` | 05:56:48 | 2.56% | 4.67 MiB / 15.23 GiB |
| `gw-wave2-redis-20261011t045752` | **所有权未知：无 Compose project/service/config 标签** | `redis:7-alpine` | 05:09:56 | 0.30% | 4.26 MiB / 15.23 GiB |
| `gw-prod-hardening-20261011t021712-gateway-1` | Gateway；`[Gateway 仓库]\deploy\wave5\compose.yaml` + `[Gateway 测试产物]\production-hardening-20261011T021712Z\compose-resource-override.yaml` | `mini-llm-gateway:ph-lock-timeout-final-4d067cc` | 03:40:46 | 0.17% | 65.16 MiB / 1 GiB |
| `gw-prod-hardening-20261011t021712-postgres-1` | 同上 | `postgres:16-alpine` | 03:32:28 | 0.00% | 23.33 MiB / 1 GiB |
| `gw-prod-hardening-20261011t021712-redis-1` | 同上 | `redis:7-alpine` | 03:13:59 | 3.01% | 5.74 MiB / 512 MiB |
| `strategyops-finalization-api-1` | StrategyOps；主配置 `[StrategyOps 仓库]\docker-compose.finalization.yml` + `[StrategyOps 运行产物]\overnight-20261011\candidate-compose-override.yml` | `strategyops-finalization:rag-metrics-8105dca` | 03:04:40 | 0.13% | 103.40 MiB / 768 MiB |
| `strategyops-finalization-db-1` | StrategyOps；`[StrategyOps 仓库]\docker-compose.finalization.yml` | `postgres:16@sha256:ca0bd484…` | 03:04:40 | 0.00% | 130.20 MiB / 768 MiB |
| `strategyops-phase4-e2e-target-fault-proxy-1` | StrategyOps；`[StrategyOps 仓库]\docker-compose.phase4.yml` | `strategyops-phase4-e2e-target-fault-proxy` | 03:04:40 | 0.00% | 17.66 MiB / 15.23 GiB |
| `strategyops-phase4-e2e-rollout-target-1` | 同上 | `strategyops-phase4-e2e-rollout-target` | 03:04:40 | 0.09% | 56.58 MiB / 15.23 GiB |

`ca0bd484…` 和 `eb587db5…` 仅为便于识别的缩写显示，不是本专项镜像验收；完整值应以对应容器 Inspect 输出为准。

## 所有权与回收判断

- 24 个运行中容器中，0 个属于本 TaskLab worktree。所有 Compose 标签均指向 Gateway 或 StrategyOps；`gw-wave2-redis-20261011t045752` 没有可验证的 Compose 所有权。
- `gateway-wave4-capacity` 的 gateway 和 PostgreSQL CPU 瞬时值分别约 183% 和 186%，存在正在执行容量验证的迹象。不得在未与该项目所有者确认完成前停止。
- 最新 `strategyops-v33-validation` 刚于 12:28 UTC 启动；明显属于仍在进行的项目工作，不应停止。
- 其他旧的诊断/硬化容器占用较小，但年龄或低 CPU 不能证明已完成；必须由对应项目所有者确认。
- 停止容器后 Windows 可用 RAM 的增量无法从容器 RSS 精确推导，WSL/Docker VM 缓存也可能继续占用；每次人工停止后必须重新运行本仓库的资源门禁脚本，直到实测达到阈值。

### 人工确认后的可恢复操作（本轮没有执行）

只对项目所有者明确确认已完成的 Compose 项目操作。使用其 Compose project 名和 Inspect 中的**全部原始配置文件**；以下示例只展示形式，路径必须以当前 Inspect 标签重新确认：

```powershell
docker compose --project-name <project> -f <compose-file-1> -f <compose-override> stop
```

恢复时对同一 project 和相同文件集合运行：

```powershell
docker compose --project-name <project> -f <compose-file-1> -f <compose-override> start
```

优先只 `stop`，不使用 `down`，不触碰卷或网络。对无所有权标签的 Redis，先由用户确认它的创建者、数据用途和恢复方式；确认之前不执行停止命令。不得停止上述正在高 CPU 运行的 `gateway-wave4-capacity`，也不得停止新启动的 `strategyops-v33-validation`，除非各自所有者确认其任务已经完成。

## 恢复验收门槛

1. 用户/项目所有者决定是否停止已确认完成的其他项目实例；本专项不代替所有者做该决定。
2. 重新运行 `deployments/local-first/scripts/check-resources.ps1 -Workload container -ReportOnly` 并确认输出时间是新的。
3. 只有可用 RAM 不低于 6 GiB、TIME_WAIT 不高于 5,000、E: 空间充足，且宿主机没有明显资源压力时，才评估 PostgreSQL-only 容器测试。若 RAM 低于 4 GiB，不运行高开销编译/Race/容器 E2E。
4. 即使门禁恢复，也必须先核实 PostgreSQL 的精确 Registry digest、数据库隔离目标和测试清理边界；不能复用其他项目的数据库或更换成未验证的本地镜像。
5. 所有本地测试结果只代表本地生产相似环境，不代表真实云生产验收。
