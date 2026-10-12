# Replay Schema and Semantics

Implemented adapter: `scripts/replay/azure_llm.py`, supporting the observed Azure LLM Code and Conversation three-column CSVs. This is a deterministic workload description generator, not a runner and not a real provider benchmark.

Each JSONL row carries `schema_version`, `workload_id`, `source_dataset`, `source_record_id`, `arrival_offset_ms`, `queue`, `tenant_id`, `task_type`, observed `context_tokens` and `generated_tokens`, a duration model marker, expected outcome, seed, and provenance markers. Token counts are copied from the public trace; time scaling, queue/tenant assignment and repeat occurrence are derived/synthetic. The source does not contain prompts, provider latency, outcome, or billing data, so the adapter does not fabricate these as observed facts.

Operational contract:

- Stream input and output; do not load the full source dataset in memory.
- Require ordered source timestamps; reject malformed, negative, or out-of-range token values.
- Require an explicit seed and bounded count/output bytes; enforce no-overwrite with temporary output plus atomic no-clobber publication.
- Emit source and output SHA256; no source text is uploaded or copied into the repository.
- Repeated source cycles preserve a stable logical order and use synthetic occurrence offsets; repeated rows are not independent observations.
- `time-scale` changes replay timing; it does not turn an arrival trace into measured service duration.
- Current adapter does not have checkpoint/resume, real-time rate pacing, deliberate error injection, idempotency-conflict injection, or task submission. 1k/10k/100k outputs are generated artifacts, not executed tasks. 1m execution was not run.

Evidence: 7/7 deterministic tests and a 1k generation are sealed under `evidence/azure-llm-replay-20261012-run02/`. Output status is `GENERATED_NOT_EXECUTED`.
