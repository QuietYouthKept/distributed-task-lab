# Azure LLM Replay Generator Evidence — Run 02

- Run ID: `azure-llm-replay-20261012-run02`
- Source commit: `d6fd35ca80fc692eb40bf99ac23196bd6200b089`
- Source tree at HEAD: `9d9610e2706c6545047303ca74ee227cf966faf0`
- Tested file snapshot was uncommitted; SHA-256 values are listed below.
- Python: `3.8.3`
- Unit command: `python scripts/replay/test_azure_llm.py -v`
- Unit result: 7 passed, 0 failed, exit 0. Includes a simulated concurrent
  destination creation; the generator did not overwrite the concurrent file.
- Generator command:
  `python scripts/replay/azure_llm.py --input E:\distributed-task-lab-datasets\azure-llm-2023\raw\AzureLLMInferenceTrace_code.csv --output E:\distributed-task-lab-datasets\processed\replays-v3\azure-llm-code-1000-final.jsonl --count 1000 --seed 20261012 --time-scale 1 --queues llm,interactive --tenants 4`
- Generator exit: 0; records: 1,000; bytes: 576,197.
- Source data SHA-256: `54e9a6d2a4bd06ba1e060304b900abbc74cbea53de96506e60fe5bb4f2277fb6`.
- Output SHA-256: `798c6736887fb81a78bc66da960c84805ed25e39b85afb90fc54893e6c031467`.
- Output status: `GENERATED_NOT_EXECUTED`.
- `azure_llm.py` SHA-256:
  `67150ddcef214afaccaef3d9af8809f93f45fec2f6e6810b30fc353cf0604540`.
- `test_azure_llm.py` SHA-256:
  `5c8f29f07d01137631ef83ada5279b3d72293a63cb9d0d4594457cc4f69ae4e9`.

`unit-tests.log` and `generator-run.log` are raw command outputs. `SHA256SUMS`
seals these logs and this README. No raw dataset content is included.
