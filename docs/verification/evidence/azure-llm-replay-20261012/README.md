# Azure LLM Replay Generator Evidence

- Run ID: `azure-llm-replay-20261012`
- Source commit: `d6fd35ca80fc692eb40bf99ac23196bd6200b089`
- Source tree at HEAD: `9d9610e2706c6545047303ca74ee227cf966faf0`
- Worktree was dirty only for this uncommitted verification package and the
  protected, untouched `FINAL_PROJECT_CLOSEOUT.md`.
- Python: `3.8.3`
- Unit command: `python scripts/replay/test_azure_llm.py -v`
- Unit result: 6 passed, 0 failed, exit 0.
- Generator command:
  `python scripts/replay/azure_llm.py --input E:\distributed-task-lab-datasets\azure-llm-2023\raw\AzureLLMInferenceTrace_code.csv --output E:\distributed-task-lab-datasets\processed\replays-v3\azure-llm-code-1000-seed-20261012.jsonl --count 1000 --seed 20261012 --time-scale 1 --queues llm,interactive --tenants 4`
- Generator exit: 0; records: 1,000; bytes: 576,197.
- Source data SHA-256: `54e9a6d2a4bd06ba1e060304b900abbc74cbea53de96506e60fe5bb4f2277fb6`.
- Output SHA-256 (manifest and independent `Get-FileHash`):
  `798c6736887fb81a78bc66da960c84805ed25e39b85afb90fc54893e6c031467`.
- Additional bounded generation runs at the same seed/config: 10,000 records,
  5,769,159 bytes, SHA-256
  `816157ccc7084a9c6f4b17726ea72d57bd3bc1d454ec57ee6eb818609f1fb7fe`; and
  100,000 records, 57,800,932 bytes, SHA-256
  `7cc878a1ff2de8d52824d5e1a49c8c0bda84187f303bf0a607ad7209e8d6484c`.
- Both larger runs exited 0 and their output hashes were independently compared
  with the generator manifests. All three outputs are
  `GENERATED_NOT_EXECUTED`.
- Output status: `GENERATED_NOT_EXECUTED`; this is not an API, Worker, Gateway,
  ArtifactStore, or performance acceptance.

The source files were modified after the recorded base commit. Their tested
snapshot hashes are:

- `scripts/replay/azure_llm.py`:
  `ce2b0c1c1cd2fa0c1b76380846a099461bd6b66f2b39267871faee938568edfe`
- `scripts/replay/test_azure_llm.py`:
  `5a16c5fc393171687d667c219f03f9d700d2afc11eee0ee1533101e9a16cc34a`

`unit-tests.log`, `generator-run.log`, `generator-10000.log`, and
`generator-100000.log` are the raw command outputs. The
adjacent `SHA256SUMS` seals those logs and this README; the source and generated
dataset hashes are recorded above. No raw dataset content is copied into this
evidence directory.
