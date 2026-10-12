# API replay runner

`runner.py` implements the API Core E2E replay mode: `CORE_E2E_BENCHMARK`. The separate Scheduler Benchmark/direct-queue mode is not implemented by this runner. It reads the generated v1 JSONL and submits one authenticated `eval/gateway` shard per row through `/v1/tasks`, then polls `/v1/tasks/{id}` to a terminal status. It does not write directly to PostgreSQL, fabricate a tenant identity, or use a mock handler. The API/Worker must be configured in the isolated `staging-sim` profile; production execution policy remains deny-all.

The Azure trace supplies token counts and relative timestamps, not evaluation examples or service duration. TaskLab's current gateway payload does not accept per-row token counts. Consequently this runner replays arrival timing, queue, and case count; token counts/tenant IDs remain source metadata. Each task runs one gateway case against an operator-prestaged dataset object. Gateway and provider behavior remain simulated; this is not an Azure model benchmark.

Before running, generate a JSONL+manifest with `azure_llm.py`, stage a dataset object into the isolated ArtifactStore, and obtain a short-lived token for one authorized staging-sim operator. Keep the token only in the named environment variable; do not put it on the command line. For example, from PowerShell:

```powershell
$env:TASKLAB_BEARER_TOKEN = '<short-lived staging token>'
python scripts/replay/runner.py `
  --input E:\replay\azure-1k.jsonl `
  --manifest E:\replay\azure-1k.jsonl.manifest.json `
  --source-dataset E:\datasets\azure-llm.csv `
  --api-url https://localhost:18443 `
  --ca-file E:\replay\staging-ca.pem `
  --run-id run-20261012-a `
  --dataset-uri artifact://datasets/azure-1k.jsonl `
  --dataset-sha256 <staged-object-sha256> `
  --time-scale 100 `
  --submit-rate 5 `
  --concurrency 4 `
  --max-runtime 1200 `
  --checkpoint E:\replay\run-20261012-a.sqlite `
  --output E:\replay\run-20261012-a-results.jsonl
Remove-Item Env:TASKLAB_BEARER_TOKEN
```

Only loopback HTTP can be enabled for isolated local development via `--allow-http-local`; HTTPS is otherwise required. Redirects are rejected, environment proxies are disabled for API calls, TLS verification is on, response bodies are capped, API calls have timeouts, submit retry count/rate/concurrency/input size/task count/result size/arrival span are bounded. Use a new run ID and new output/checkpoint paths per experiment. Resume only with the original paths, input, run ID, endpoint and dataset identity, adding `--resume`; SQLite checkpoint rows are committed as terminal outcomes arrive. A crash between API acceptance and checkpoint commit is safe to retry because the same input ordinal uses the same idempotency key.

## Bounded soak

`soak.py` repeatedly invokes this same real API runner for an explicit 1h, 4h, or 8h duration. It requires a confirmed Run ID, clean source checkout, a short-lived staging token in the environment, and a fresh evidence directory. Each iteration gets a unique task Run ID, checkpoint, and result file. Linux `/proc` sampling enforces the 6-GiB available-memory floor, a child RSS ceiling, and an evidence-size ceiling; a failed iteration or resource breach stops the run and leaves a sealed manifest, per-iteration logs/results, resource samples, and `SHA256SUMS`. Ctrl+C stops the active child and seals an `INTERRUPTED` result. The runner does not provision infrastructure, delete tasks, or claim a soak pass unless the requested wall-clock duration completes.

This soak harness has unit coverage only until it is run against an isolated staging-sim API/Worker/Gateway/ArtifactStore stack. Any result remains simulated-provider evidence, not production capacity or reliability proof.

The summary records API-observed submit latency, database timestamps for queue-wait/E2E latency (or `null`/`UNAVAILABLE`), result and checkpoint SHA256, runner/generator hashes, source commit/tree, tracked-diff hash and dirty state. It does not contain the bearer token. `duplicate_logical_tasks` is `UNAVAILABLE`: the stable v1 response does not reveal whether enqueue returned an existing task. The checkpoint is the durable partial evidence if interrupted before JSONL export.

Run local contract tests with:

```powershell
python -m unittest discover -s scripts/replay -p 'test_*.py' -v
```

These tests use a local fake HTTP server and validate the client runner; they are not TaskLab integration or Core E2E evidence. Only a run against the isolated API + PostgreSQL + Worker + Gateway Simulator + ArtifactStore can produce meaningful `CORE_E2E_BENCHMARK` results.
