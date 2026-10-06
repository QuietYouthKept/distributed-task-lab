# LLM evaluation shards

`eval` has two separate backends. `EVAL_BACKEND=deterministic` (the default)
is a reproducible pseudo evaluator for CI, load tests, and fault injection. It
must never be described as model quality or provider throughput.

`EVAL_BACKEND=gateway` calls `POST $EVAL_GATEWAY_URL/v1/evaluations` with a
stable request ID: `run_id:case_id:model_profile:prompt_version`. The gateway
must deduplicate that request ID and return `status` (`passed` or `failed`),
`score`, `input_tokens`, `output_tokens`, and `cost_usd`.

Gateway tasks require v2 provenance: `run_id`, `dataset_version`,
`dataset_uri`, `dataset_sha256`, `case_start`, `case_end`, `model_profile`,
`prompt_version`, `retriever_version`, and `evaluator_schema_version`. The
range is end-exclusive and capped by `EVAL_MAX_CASES_PER_SHARD` (50 by default).

Every completed case is fsynced to a partial JSONL receipt before the next call.
Final files are atomically published under:

```text
$RAG_ARTIFACT_DIR/eval/<run-id>/<fingerprint>/manifest.json
$RAG_ARTIFACT_DIR/eval/<run-id>/<fingerprint>/case-results.jsonl
$RAG_ARTIFACT_DIR/eval/<run-id>/<fingerprint>/summary.json
$RAG_ARTIFACT_DIR/eval/<run-id>/<fingerprint>/failures.jsonl
```

Consequently a reclaim resumes from receipts. A crash after a provider response
but before its receipt is handled by the gateway's stable request ID. API keys
come only from `EVAL_GATEWAY_API_KEY`; they are never written to task data,
results, artifacts, or logs. Optional `EVAL_*_ALLOWLIST` variables restrict
version fields. `EVAL_MAX_COST_USD` defaults to $10.
