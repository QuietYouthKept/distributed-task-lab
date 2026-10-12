# Local Dataset Asset Inventory

Inventory captured 2026-10-12 from E: only. Large archives were not extracted;
the named Parquet files were not decoded. No dataset contents were uploaded.
Hashes are SHA-256 unless a field is explicitly `NOT_COMPUTED`.

## Audited workload datasets

| Asset | Local file(s), bytes, SHA-256 | Actual structure/count | Integrity and quality | Provenance, license, citation | Workload use / limits |
|---|---|---|---|---|---|
| Azure LLM 2023 Code | `E:\distributed-task-lab-datasets\azure-llm-2023\raw\AzureLLMInferenceTrace_code.csv`, 320,117, `54e9a6d2a4bd06ba1e060304b900abbc74cbea53de96506e60fe5bb4f2277fb6` | 8,819 rows; `TIMESTAMP,ContextTokens,GeneratedTokens` | 0 invalid/missing, 0 duplicate timestamps, 0 exact duplicates; ordered timestamps | Microsoft Azure Public Dataset. CC-BY attribution; cite Splitwise (ISCA 2024). Official docs say collected Nov 11, while local timestamps are Nov 16; timezone unspecified. | Input/output-size and arrival replay; no prompt text or service duration. `scripts/replay/azure_llm.py` supports bounded deterministic conversion. |
| Azure LLM 2023 Conversation | `E:\distributed-task-lab-datasets\azure-llm-2023\raw\AzureLLMInferenceTrace_conv.csv`, 719,188, `2f1e5b666d4e3055fdbba98598ce2ec307767b9064e03e2fa46676dbcc7d0bf8` | 19,366 rows; same schema | 0 invalid/missing, 0 duplicate timestamps, 0 exact duplicates; ordered timestamps | Same source/terms/citation as Code. | Same limits as Code. |
| Azure Functions 2021 invocation | RAR `E:\distributed-task-lab-datasets\azure-functions-2021\raw\AzureFunctionsInvocationTraceForTwoWeeksJan2021.rar`, 18,444,269, `8bb5d1c82e89e467062b6582996232df24edfebadbaf3287a8c985ca178ba92d`; extracted `E:\distributed-task-lab-datasets\azure-functions-2021\extracted\AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt`, 305,388,316, `d56368ef194baa8d418304bd2f87cca67668ced0d117bd89ad4ef3cf836457d2` | 1,980,951 records; comma-delimited `app,func,end_timestamp,duration`; 1,980,951 valid field-count rows | 363,528 source-order timestamp regressions; min/max end timestamp 0.0668–1,209,599.6945; duration 0–578.62. No empty/malformed rows. | Microsoft Azure Public Dataset; CC-BY Attribution on the dataset page; cite Zhang et al., SOSP 2021. Unit conflict: the dataset-specific page says seconds, while the repository overview currently says milliseconds; two-week range strongly resembles seconds, but adapter must not normalize until this conflict is resolved. | Invocation arrival/completion and long-tail workload after unit semantics are fixed. Trace rows are invocations, not complete HTTP request payloads. |
| Alibaba Cluster Trace 2018 `batch_task` | `E:\distributed-task-lab-datasets\alibaba-batch-2018\raw\batch_task.tar.gz`, 130,310,549, `7c4b32361bd1ec2083647a8f52a6854a03bc125ca5c202652316c499fbf978c6`; one regular member `batch_task.csv`, 802,261,444 uncompressed | 14,295,731 headerless rows; field order `task_name,instance_num,job_name,task_type,status,start_time,end_time,plan_cpu,plan_mem` | 0 wrong-width rows; 61,857 rows with empty cells / invalid numeric fields; 23,517 `end_time < start_time`; start/end range 0–772,823; official units are seconds from trace start. | Official Alibaba Open Cluster Trace 2018 page; its published `batch_task.tar.gz` checksum matches local bytes. The page requests survey registration but the inspected page has no explicit dataset license text; do not assume reuse terms. | Job/task arrival and duration candidates. `task_name` encodes DAG dependencies; current TaskLab has no DAG scheduler, so mapping only independent tasks loses dependency semantics. |
| BEIR SciFact | ZIP `E:\distributed-task-lab-datasets\beir-scifact\raw\scifact.zip`, 2,816,079, `536e14446a0ba56ed1398ab1055f39fe852686ecad24a6306c80c490fa8e0165`; `downloads\scifact.zip` is byte-identical | 5,183 corpus docs, 1,109 queries, 919 train qrels, 339 test qrels | ZIP CRC good; no unsafe paths; no duplicate corpus/query IDs or qrels pairs; all qrels query/document IDs resolve. `corpus.jsonl` fields `_id,title,text,metadata`; query `_id,text,metadata`; qrels `query-id,corpus-id,score`. | BEIR/UKP release and AllenAI SciFact paper/repo. BEIR explicitly disclaims granting or verifying rights; underlying data permissions must be checked before redistribution. Cite the SciFact and BEIR papers. | Retrieval-task scheduling and artifact persistence only. Without an actual retrieval/ranking system, not a retrieval-quality benchmark. |
| BEIR FiQA | `E:\fiqa.zip`, 17,948,027, `32c7df99ed21252a8d245ee0c44c4a133570d92ce2b3ad02` | 57,638 docs; 6,648 queries; qrels: train 14,166, dev 1,238, test 1,706 | ZIP CRC good; no unsafe paths, duplicate IDs/pairs, malformed records, or qrels references missing from corpus/query. | BEIR-prepared archive; BEIR states users are responsible for dataset-specific permissions and citation. Original FiQA terms still require review. | Finance-domain query-size/task batching only; retrieval quality requires a real retriever. |
| BEIR TREC-COVID | `E:\trec-covid.zip`, 73,876,720, `120f42a7864d2214234537733c0d2c6684e42fdfafff2c5eacf98afca6656aa0` | 171,332 docs; 50 queries; 66,336 test qrels | ZIP CRC good; no unsafe paths, duplicate IDs/pairs, malformed rows, or missing qrels references. Scores include -1, 0, 1, 2; retain the source labels. | BEIR-prepared archive; source-specific TREC-COVID corpus/qrels terms were not established by the local archive. Verify before redistribution and cite BEIR plus the original track/data owners. | Retrieval workload and result persistence; not a scheduler quality metric. |
| RAGTruth | `E:\RAGTruth-main.zip`, 7,396,664, `41254750e163aed1df999c994ebc552fdfd8d059a606dfe3195fa405c6daf21a` | 2,965 source-info records; 17,790 response records; exactly six responses per source; response split 15,090 train / 2,700 test | ZIP CRC good, paths safe, JSONL parses; 0 duplicate IDs; all response `source_id` references resolve. Label spans are present in response records. | Official ParticleMedia RAGTruth repo explains schema and paper citation. Its repository MIT license was observed, but that alone is not treated as a data-content license grant; confirm data terms before redistribution. | RAG evaluation-task queue and audit payloads; no model training or quality benchmark is claimed here. Keep response text local pending privacy/content review. |

The manifests under the external E: dataset directory are inputs, not committed
TaskLab evidence. Azure LLM, SciFact, and Alibaba hashes were independently
recomputed during this audit; the same is true for the FiQA, TREC-COVID, and
RAGTruth archives.

## Present but not semantically audited

| Local asset | Size / observed format | Current status and reason |
|---|---:|---|
| `E:\DocLayNet_core.zip` | 30,012,083,650 bytes; ZIP extension | Metadata only; no central-directory scan, extraction, or image decode due 30 GB size and unverified contents. Source/version/license, image/annotation pairs, splits, SHA-256 and corruption status remain `NOT_VERIFIED`. |
| `E:\val.parquet` | 1,190,648,470 bytes | Source/schema/row groups/row count unknown. No `pyarrow`, `duckdb`, or `fastparquet` runtime is installed. No decode attempted. |
| `E:\test.parquet` | 3,995,461,246 bytes | Same; do not infer a dataset name from neighboring files. |
| `E:\dataset.zip` | 16,838,830 bytes; 824 archive entries, 809 files, 27,879,766 uncompressed bytes | 298 train and 100 test PNG/JSON pairs with matching stems; 10 metadata/no-extension files; safe relative paths, no symlinks, no extraction/image decode/annotation text read. Dataset identity, official source and license have not been established. Do not call it FUNSD or another named benchmark. |

At initial inventory time the tree contained 42 files / 783,327,186 bytes under
`E:\distributed-task-lab-datasets`, including raw inputs and prior processed
replays; bounded 1k/10k/100k replay artifacts were subsequently added under
`processed/replays-v3`. Previous `processed/replays` and `processed/replays-v2`
outputs have manifests, but no converter source SHA was found in TaskLab; they
are not accepted as reproducible current-code output. The new v3 outputs have
source hashes, output hashes and deterministic generator test evidence.

## Rights and privacy handling

No data was copied into Git or sent to any service. Public-source status does
not automatically mean all text, labels or derived data may be republished.
Before cloud upload, check each source's explicit terms, paper citation, privacy
and retention requirements. For DocLayNet, Parquet, and the unnamed form archive,
source/terms must be established before any upload or public sharing.

## Official references

- [Azure Public Dataset: Azure LLM Inference Trace 2023](https://github.com/Azure/AzurePublicDataset/blob/master/AzureLLMInferenceDataset2023.md)
- [Azure Public Dataset: Azure Functions Invocation Trace 2021](https://github.com/Azure/AzurePublicDataset/blob/master/AzureFunctionsInvocationTrace2021.md)
- [Alibaba Open Cluster Trace 2018 documentation](https://github.com/alibaba/clusterdata/blob/master/cluster-trace-v2018/trace_2018.md)
- [BEIR dataset catalog and license disclaimer](https://github.com/beir-cellar/beir/wiki/Datasets-available)
- [AllenAI SciFact](https://github.com/allenai/scifact)
- [ParticleMedia RAGTruth dataset documentation](https://github.com/ParticleMedia/RAGTruth)
