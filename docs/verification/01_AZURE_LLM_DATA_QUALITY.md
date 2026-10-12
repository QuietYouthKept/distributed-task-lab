# Azure LLM Inference Trace 2023: Local Data Audit

Audit source: local E: dataset files, streamed on 2026-10-12.
Audited repository baseline: `d6fd35ca80fc692eb40bf99ac23196bd6200b089` / Tree `9d9610e2706c6545047303ca74ee227cf966faf0`.

## Provenance and limits

Microsoft's Azure Public Dataset documentation describes two November 2023
LLM-service traces, gives the three-column schema, states CC-BY Attribution,
and asks users to cite the Splitwise ISCA 2024 paper. The public documentation
says collection was on November 11, while the local CSV timestamp values fall on
November 16. The documentation does not specify a timezone for these fields.
This mismatch is retained as a provenance warning; timestamps are treated as
naive source values, not converted to UTC or claimed to be wall-clock request
times. The traces contain sizes, not prompt contents or measured service
durations. Therefore token-derived execution time and LLM serving throughput
must not be inferred from this dataset.

Official source: [Azure Public Dataset, Azure LLM Inference Trace 2023](https://github.com/Azure/AzurePublicDataset/blob/master/AzureLLMInferenceDataset2023.md).
Required citation: Pratyush Patel et al., “Splitwise: Efficient generative LLM
inference using phase splitting,” ISCA 2024. Follow the source's CC-BY
attribution terms if redistributing derived material. Do not publish raw data
without reviewing its source terms and the project's privacy policy.

## Integrity and row quality

| File | Bytes | SHA-256 | Records | Schema | Invalid/missing | Duplicate timestamp / exact rows | Time range |
|---|---:|---|---:|---|---:|---:|---|
| Code | 320,117 | `54e9a6d2a4bd06ba1e060304b900abbc74cbea53de96506e60fe5bb4f2277fb6` | 8,819 | `TIMESTAMP,ContextTokens,GeneratedTokens` | 0 | 0 / 0 | `2023-11-16 18:17:03.9799600` – `19:14:19.9280160` |
| Conversation | 719,188 | `2f1e5b666d4e3055fdbba98598ce2ec307767b9064e03e2fa46676dbcc7d0bf8` | 19,366 | same | 0 | 0 / 0 | `2023-11-16 18:15:46.6805900` – `19:14:08.4025270` |

Both CSVs were read with a strict three-column CSV parser. All timestamps parsed
as `YYYY-MM-DD HH:MM:SS.fffffff`; their seven fractional digits were preserved
at 100 ns precision for ordering and inter-arrival calculations. Rows are
nondecreasing by timestamp. Token values are non-negative integers and all
observed values are nonzero. The `raw` and `downloads` copies have the same
sizes and SHA-256 values, so those are duplicate copies, not additional data.

Percentiles below use linear interpolation (`(n-1) * p`). Request rates are
derived from timestamp counts and describe this trace sample only.

## Token distribution

| Trace | Metric | Min | P50 | P90 | P95 | P99 | Max | Mean |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| Code | Context tokens | 3 | 1,469 | 5,187.6 | 7,303.3 | 7,436 | 7,437 | 2,047.85 |
| Code | Generated tokens | 6 | 13 | 55 | 90 | 251.46 | 1,899 | 27.88 |
| Code | Total tokens | 12 | 1,493 | 5,207.8 | 7,331.3 | 7,461.82 | 7,841 | 2,075.73 |
| Conversation | Context tokens | 2 | 1,020 | 2,734.5 | 4,083 | 4,142 | 14,050 | 1,154.70 |
| Conversation | Generated tokens | 7 | 129 | 424 | 451 | 601 | 1,000 | 211.13 |
| Conversation | Total tokens | 64 | 1,412 | 2,819.5 | 4,136 | 4,259 | 14,089 | 1,365.82 |

Code has much larger prompt contexts and much shorter generated responses in
this sample (mean context/generated ratio 163.54 versus 12.82). There are 1,241
Code rows with context over 4,096 tokens and 2 with generated output over 1,024;
Conversation has 402 contexts over 4,096 and no generated outputs over 1,024.

## Arrival behavior

| Trace | Span | Inter-arrival p50 / p95 / p99 | Mean gap | Max gap | Mean over full span | Mean per occupied second | Peak per second |
|---|---:|---:|---:|---:|---:|---:|---:|
| Code | 57m 15.948056s | 63.798 / 400.776 / 2,205.070 ms | 389.652 ms | 217,168.952 ms | 2.567 req/s | 9.649 req/s | 67 req/s |
| Conversation | 58m 21.721937s | 118.424 / 565.194 / 889.671 ms | 180.827 ms | 4,314.579 ms | 5.530 req/s | 5.567 req/s | 19 req/s |

There are 914 Code seconds and 3,479 Conversation seconds containing at least
one event. Per-second peaks are counts in source timestamp buckets; they are not
hardware throughput measurements. Conversation has denser arrivals across its
full interval, while Code has larger bursts and a much longer maximum idle gap.

For one-second buckets spanning each trace, Code has 2,523 empty seconds, a
maximum 216-second run of empty buckets, and population variance 33.88
requests²/s²; Conversation has 24 empty seconds, a maximum 3-second empty run,
and variance 7.34. Defining a burst explicitly as a run of buckets with at least
5 requests/s gives Code 183 bursts (730 seconds total, longest 26 seconds) and
Conversation 697 bursts (2,175 seconds total, longest 30 seconds). At the
stricter 10 requests/s threshold, the longest runs are 12 seconds and 3 seconds
respectively. Sliding 60-second windows peak at 622 Code records and 506
Conversation records. These bucket/window statistics are derived from the
released sample and depend on the stated threshold/window definition.

## Replay semantics

The new `scripts/replay/azure_llm.py` emits a generic JSONL workload record, not
an HTTP request. Token counts are `OBSERVED`; row identity and relative arrival
offset are `DERIVED`; tenant, queue assignment, seed, and expected outcome are
`SYNTHETIC`. Repeated passes beyond the source row count are also synthetic.
There is no duration model because the source does not include execution time.
All consumers must still use normal authentication, admission, payload
validation, and execution-policy checks. Generation is bounded by an explicit
record count and byte ceiling and refuses to overwrite existing output.

Generator unit tests: `python scripts/replay/test_azure_llm.py -v` (7 tests,
Python 3.8.3). These test deterministic bytes, provenance, repeat-cycle
ordering, malformed/out-of-order input, byte limits, and no-overwrite behavior,
including a simulated concurrent output creation.
The real-data replay acceptance generated separate 1,000-, 10,000-, and
100,000-record Code workloads at seed `20261012` and scale `1`. Their output
SHA-256 values are respectively `798c6736887fb81a78bc66da960c84805ed25e39b85afb90fc54893e6c031467`,
`816157ccc7084a9c6f4b17726ea72d57bd3bc1d454ec57ee6eb818609f1fb7fe`, and
`7cc878a1ff2de8d52824d5e1a49c8c0bda84187f303bf0a607ad7209e8d6484c`. The
100,000-record output is 57,800,932 bytes and makes 12 passes over 8,819 source
rows; repeated source records are explicitly synthetic occurrences. The 1,000-
record output was independently parsed for schema, token values, count and
nondecreasing arrivals. The first 1k/10k/100k batch logs are under
`docs/verification/evidence/azure-llm-replay-20261012/`; the final source
snapshot's 7-test run and 1k replay are sealed under
`docs/verification/evidence/azure-llm-replay-20261012-run02/`. Generation alone
is not an execution or performance result.
