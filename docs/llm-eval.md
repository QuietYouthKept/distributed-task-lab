# LLM evaluation shards

`eval` tasks become a deterministic eval workload when the payload contains
`dataset_version`, `model_profile`, `prompt_version`, `run_id`, `case_start`,
and `case_end` (an end-exclusive range). The worker writes one result artifact
per immutable input fingerprint under `RAG_ARTIFACT_DIR/eval/<run_id>/`.

The shipped evaluator intentionally calculates reproducible pseudo-results: it
has no model-provider account or hidden network dependency. Its purpose is to
exercise the queue contract faithfully. Replace the per-case logic with a real
gateway while retaining the fingerprint, result URI, token/cost metrics, and
lookup-before-write behavior.

Retrying a shard reads the same artifact and reports `idempotent_reused=true`.
Consumers can reconcile a run using `run_id`, `result_fingerprint`, and
`case_results_uri`; no retry creates another final-shard artifact.

See [`examples/payloads/eval_shard.json`](../examples/payloads/eval_shard.json).
