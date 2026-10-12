# Evidence Integrity Acceptance

Audit generated UTC: 2026-10-11T12:18:04.9344134Z
Evidence root: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\deep-engineering`
Matrix: `docs\production-readiness\VERIFICATION_MATRIX.json`
Manifest rows audited: 117; rows with issues: 10.

This is an integrity audit, not a new test execution. Historical source commits and trees remain unchanged. A historical PASS is scoped to the exact recorded command, test counters, source inventory, commit/tree, and files sealed by that run.

## Per-run results

| Run ID | Suite | Status | Source commit | Tree | Dirty | Inventory files | Exit | Tests | Pass | Fail | Skip | Hashes valid | Issues |
|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---|---|
| 20261011T060427988Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True |  | 0 | 0 | 0 | 0 | 0 | True | dirty_source_inventory_missing |
| 20261011T060428323Z | unit | FAIL | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True |  | 1 | 0 | 0 | 0 | 0 | True | dirty_source_inventory_missing |
| 20261011T060614491Z | unit | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True |  | 0 | 28 | 28 | 0 | 0 | True | dirty_source_inventory_missing |
| 20261011T060725858Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True |  | 0 | 0 | 0 | 0 | 0 | True | dirty_source_inventory_missing |
| 20261011T060726345Z | unit | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True |  | 0 | 115 | 115 | 0 | 0 | True | dirty_source_inventory_missing |
| 20261011T060844810Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 28 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T060845426Z | unit | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 28 | 0 | 115 | 115 | 0 | 0 | True |  |
| 20261011T100149186Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 28 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T100305783Z | race | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 28 | 0 | 115 | 115 | 0 | 0 | True |  |
| 20261011T100336340Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 28 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T100455342Z | integration | DEPENDENCY_BLOCKED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 30 | 3 | 0 | 0 | 0 | 0 | True | test_command_missing |
| 20261011T100455719Z | e2e | RESOURCE_LIMITED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 30 | 3 | 0 | 0 | 0 | 0 | True | test_command_missing |
| 20261011T100501556Z | security | DEPENDENCY_BLOCKED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 30 | 3 | 0 | 0 | 0 | 0 | True | test_command_missing |
| 20261011T100602438Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 8fce4357ed52084629b6a9a23b007f98b6b3858b | True | 31 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T101242969Z | static | PASS | 47974b3c7e11447e60eec6afda9ab40513e65a51 | b9c25b4ac1421b620ed03fa33a981128a99a5d90 | True | 6 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T101607464Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 3 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T101738309Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 3 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T101808311Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 3 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T101919330Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 8 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T101919757Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 8 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T102011958Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 11 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T102012348Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 11 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T102114274Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 11 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T102114664Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 11 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T102154311Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 11 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T102154716Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 2ce803b255a8cc0356e468a6fc188c5f319a2072 | True | 11 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T102427928Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 352df4a15beb3b89332f57247ba1f3ea37ce37d6 | True | 1 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T102540809Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 352df4a15beb3b89332f57247ba1f3ea37ce37d6 | True | 1 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T102622442Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 352df4a15beb3b89332f57247ba1f3ea37ce37d6 | True | 1 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T102917751Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 352df4a15beb3b89332f57247ba1f3ea37ce37d6 | True | 5 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T103019956Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 352df4a15beb3b89332f57247ba1f3ea37ce37d6 | True | 7 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T103054400Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 352df4a15beb3b89332f57247ba1f3ea37ce37d6 | True | 7 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T103202147Z | retention-fixture | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | cd632dead6670a437e2c74c01b91d538d5c58108 | True | 1 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T103229088Z | retention-fixture | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | cd632dead6670a437e2c74c01b91d538d5c58108 | True | 1 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T103233171Z | static | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | cd632dead6670a437e2c74c01b91d538d5c58108 | True | 1 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T103317845Z | retention-fixture | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | cd632dead6670a437e2c74c01b91d538d5c58108 | True | 5 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T103322262Z | static | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | cd632dead6670a437e2c74c01b91d538d5c58108 | True | 5 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T103347962Z | static | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | cd632dead6670a437e2c74c01b91d538d5c58108 | True | 7 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T111357873Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 3 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T111400325Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 3 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T111554553Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 8 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T111557353Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 8 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T112052387Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 14 | 0 | 10 | 10 | 0 | 0 | True |  |
| 20261011T112054430Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 14 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T112212586Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 18 | 0 | 7 | 7 | 0 | 0 | True |  |
| 20261011T112214771Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 18 | 0 | 10 | 10 | 0 | 0 | True |  |
| 20261011T112217192Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 18 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T112219594Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 18 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T112537861Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T112540456Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 7 | 7 | 0 | 0 | True |  |
| 20261011T112542631Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 10 | 10 | 0 | 0 | True |  |
| 20261011T112545221Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T112548753Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T112558343Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T112739419Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 8 | 8 | 0 | 0 | True |  |
| 20261011T112741846Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T112744592Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 10 | 10 | 0 | 0 | True |  |
| 20261011T112747665Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T112750931Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T112803914Z | evidence-audit | FAIL | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 22 | 1 | 59 | 51 | 8 | 0 | True |  |
| 20261011T113028538Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 24 | 0 | 13 | 13 | 0 | 0 | True |  |
| 20261011T113034364Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 24 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T113228300Z | supply-chain-static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 4 | 4 | 0 | 0 | True |  |
| 20261011T113539985Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T113542764Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T113545207Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 13 | 13 | 0 | 0 | True |  |
| 20261011T113547912Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T113550721Z | supply-chain-static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 4 | 4 | 0 | 0 | True |  |
| 20261011T113553245Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 29 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T113954997Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 31 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T114044981Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 31 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T114331778Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T114332176Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T114333732Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 13 | 13 | 0 | 0 | True |  |
| 20261011T114334891Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T114336119Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T114337682Z | supply-chain-static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 4 | 4 | 0 | 0 | True |  |
| 20261011T114339053Z | retention-fixture | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T114340591Z | evidence-audit | FAIL | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 32 | 1 | 78 | 63 | 15 | 0 | True |  |
| 20261011T114523729Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 33 | 0 | 16 | 16 | 0 | 0 | True |  |
| 20261011T114525500Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | ee8921362e9d13ae0d85cd120ff937a256a2e029 | True | 33 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T114644279Z | static | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T114644762Z | evidence-integrity | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 15 | 15 | 0 | 0 | True |  |
| 20261011T114646483Z | compose-cleanup | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 16 | 16 | 0 | 0 | True |  |
| 20261011T114648096Z | database-safety | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 9 | 9 | 0 | 0 | True |  |
| 20261011T114649283Z | backup-manifest | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T114650665Z | supply-chain-static | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 4 | 4 | 0 | 0 | True |  |
| 20261011T114652003Z | retention-fixture | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | ce6b4a5feb602e27bc71114b8fe7269970810420 | False | 0 | 0 | 12 | 12 | 0 | 0 | True |  |
| 20261011T120008856Z | e2e | RESOURCE_LIMITED | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 35158bca5b0cd0768986da5f123e45af689d11ad | True | 2 | 3 | 0 | 0 | 0 | 0 | True | test_command_missing |
| 20261011T120124069Z | evidence-integrity | PASS | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 35158bca5b0cd0768986da5f123e45af689d11ad | True | 8 | 0 | 16 | 16 | 0 | 0 | True |  |
| 20261011T120125787Z | static | PASS | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 35158bca5b0cd0768986da5f123e45af689d11ad | True | 8 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120126088Z | e2e | RESOURCE_LIMITED | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 35158bca5b0cd0768986da5f123e45af689d11ad | True | 8 | 3 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120239543Z | evidence-integrity | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | False | 0 | 0 | 16 | 16 | 0 | 0 | True |  |
| 20261011T120241222Z | static | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | False | 0 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120241494Z | e2e | RESOURCE_LIMITED | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | False | 0 | 3 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120525107Z | static | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120525622Z | evidence-integrity | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 17 | 17 | 0 | 0 | True |  |
| 20261011T120527345Z | unit | RESOURCE_LIMITED | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 3 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120533380Z | vet | RESOURCE_LIMITED | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 3 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120623118Z | static | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120623638Z | evidence-integrity | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 17 | 17 | 0 | 0 | True |  |
| 20261011T120625397Z | unit | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 227 | 227 | 0 | 0 | True |  |
| 20261011T120746728Z | vet | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T120821725Z | race | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 5af5a3562d3c3f1956163889dcb72e9d71ff1414 | True | 6 | 0 | 227 | 227 | 0 | 0 | True |  |
| 20261011T121021874Z | unit | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 845aa22d268728a603611595693bfe0444a7dedd | True | 4 | 0 | 227 | 227 | 0 | 0 | True |  |
| 20261011T121044229Z | vet | RESOURCE_LIMITED | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 845aa22d268728a603611595693bfe0444a7dedd | True | 4 | 3 | 0 | 0 | 0 | 0 | True |  |
| 20261011T121050008Z | race | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 845aa22d268728a603611595693bfe0444a7dedd | True | 4 | 0 | 227 | 227 | 0 | 0 | True |  |
| 20261011T121231709Z | static | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 845aa22d268728a603611595693bfe0444a7dedd | True | 5 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T121232271Z | evidence-integrity | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 845aa22d268728a603611595693bfe0444a7dedd | True | 5 | 0 | 18 | 18 | 0 | 0 | True |  |
| 20261011T121456412Z | unit | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | False | 0 | 0 | 227 | 227 | 0 | 0 | True |  |
| 20261011T121525531Z | race | RESOURCE_LIMITED | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | False | 0 | 3 | 0 | 0 | 0 | 0 | True | test_command_missing |
| 20261011T121540263Z | static | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | False | 0 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T121540764Z | evidence-integrity | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | False | 0 | 0 | 18 | 18 | 0 | 0 | True |  |
| 20261011T121542624Z | vet | RESOURCE_LIMITED | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | False | 0 | 3 | 0 | 0 | 0 | 0 | True |  |
| 20261011T121728700Z | static | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | True | 8 | 0 | 0 | 0 | 0 | 0 | True |  |
| 20261011T121729277Z | evidence-integrity | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | True | 8 | 0 | 18 | 18 | 0 | 0 | True |  |
| 20261011T121731548Z | race | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 82bed902a347cd4196e6d902c30af24a44444106 | True | 8 | 0 | 227 | 227 | 0 | 0 | True |  |

## Status counts

| Status | Count |
|---|---:|
| DEPENDENCY_BLOCKED | 2 |
| FAIL | 3 |
| PASS | 103 |
| RESOURCE_LIMITED | 9 |

## Interpretation

- A PASS with zero executed tests is static/tooling evidence only; it does not establish functional acceptance.
- Historical Unit/Race results remain attached to their original commit and tree; they are not evidence for a later HEAD.
- The earliest five runs predate source-inventory capture. Their `dirty_at_start=true` state cannot be tied to a changed-file list; those historical rows are intentionally flagged and have not been rewritten.
- The new runner self-tests separately prove status classification, dirty-source inventory, closed-log sealing, tamper detection, and refusal to overwrite existing evidence. They do not prove external service behavior.
- If any row has an issue, the matrix and source artifacts must be reconciled before claiming a clean audit.

## Audit summary

AUDIT_STATUS=FAIL
Issues: 10
