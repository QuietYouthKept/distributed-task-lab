# Sanitized publication and evidence preservation

This publication snapshot is derived from source commit `161c98ee95d4953bdd92adb8716288a8714758d5` (tree `5e26aa6a9d0ebeb6b9aef6a44f2db63702d82652`). It replaces machine-specific path disclosure with portable placeholders. Original source history and original Evidence files remain unchanged on the local `codex/production-readiness-p0-p1` branch; this snapshot is a redacted derivative and must not be represented as byte-identical to that source.

The public snapshot is intended to be a single child of verified baseline `64fd7bb986984cb6abe0a15295fda319c31049b5`. This preserves the complete current source/evidence tree while avoiding publication of the 68 commits not reachable from the existing public refs. The unredacted local branch remains the audit backup. The protected untracked `FINAL_PROJECT_CLOSEOUT.md` and `replay-runner-20261012-run01` evidence are not part of this snapshot.

Redaction mapping:

- Machine-specific user-profile, repository-root, and artifact-root prefixes become `<REDACTED_USER_PROFILE>`, `<REDACTED_REPOSITORY_ROOT>`, and `<REDACTED_LOCAL_ARTIFACT_ROOT>` in documents and logs.
- PowerShell runners use `TASKLAB_ARTIFACT_ROOT`, defaulting to the OS temporary directory. `collect-static-evidence.ps1` accepts `GO_EXE` or resolves `go` from `PATH` instead of embedding a user-specific toolchain path.

Historical log files containing these paths are published only as redacted derivatives. Their original SHA256 values remain associated with the immutable source history and any pre-existing manifests. The machine-readable [redaction manifest](evidence/publication-redaction-20261012/manifest.json) records source and derivative SHA256 values for every modified tracked file, including the Evidence logs, so reviewers can distinguish original Evidence from its privacy-scrubbed publication copy. A checksum mismatch against a historical manifest is expected only for the listed redacted derivatives; other Evidence is not intentionally altered.

On Windows, set `TASKLAB_ARTIFACT_ROOT` to a private E-drive location before invoking runners that retain an E-drive safety guard. The repository no longer embeds the user's concrete artifact directory.
