# Payload contract

Every enqueue request uses envelope version `1`, requires `payload.kind` to
equal the task `type`, and rejects unknown `payload.data` fields. Every mode is
explicit; a typo never falls back to a mock handler.

| Task type | `execution_mode` | Required data | Important bounds |
|---|---|---|---|
| `rag_ingestion` | `mock` | `document_id` | strings are bounded; mock controls are non-negative integers |
| `rag_ingestion` | `real` | `document_id`, `object_key` | `object_key` is resolved below the configured trusted root; paths, URLs, traversal and symlink escape are rejected |
| `eval` | `mock` | `eval_set_id`, `model_profile` | `sample_limit` is 1–100000 when present |
| `eval` | `deterministic` | `dataset_version`, `model_profile`, `prompt_version`, `run_id`, `case_start`, `case_end` | range is end-exclusive and must be non-empty |
| `eval` | `gateway` | deterministic fields plus data `version=2`, `dataset_uri`, `dataset_sha256`, `retriever_version`, `evaluator_schema_version` | dataset URI must use `artifact://`; SHA-256 is exactly 64 hex characters; worker applies its configured shard cap and allowlists |
| `benchmark` | `mock` | `benchmark_name` | optional concurrency is 1–100000 |
| `strategy_replay` | `replay` | `proposal_id`, schema versions, simulator and execution fingerprints, `strategy_key`, `candidate_config` | `candidate_config` must be an object; optional case count is bounded |
| `payment`, `notification` | `effect` | matching `operation`, `operation_id`, `target` | operation must equal the task type; simulation flags are booleans |

The executable source of truth is
[`internal/domain/taskpayload/registry.go`](../internal/domain/taskpayload/registry.go).
`TestCheckedInPayloadExamplesMatchRegistry` validates every checked-in JSON
example against it during normal test runs.
