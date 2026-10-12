#!/usr/bin/env python3
"""Bounded repeated API replay soak runner for isolated staging-sim deployments."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path


DURATIONS = {"1h": 3600, "4h": 14400, "8h": 28800}
RUN_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$")


class SoakError(Exception):
    pass


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def linux_memory_mib(meminfo: Path = Path("/proc/meminfo")) -> int:
    for line in meminfo.read_text(encoding="ascii").splitlines():
        if line.startswith("MemAvailable:"):
            return int(line.split()[1]) // 1024
    raise SoakError("MemAvailable is unavailable; no replay started")


def linux_rss_mib(pid: int, proc_root: Path = Path("/proc")) -> int:
    status = (proc_root / str(pid) / "status").read_text(encoding="ascii")
    for line in status.splitlines():
        if line.startswith("VmRSS:"):
            return int(line.split()[1]) // 1024
    raise SoakError("child VmRSS is unavailable; no further iteration will start")


def directory_bytes(path: Path) -> int:
    total = 0
    for item in path.rglob("*"):
        if item.is_file() and not item.is_symlink():
            total += item.stat().st_size
    return total


def git_value(root: Path, expression: str) -> str:
    return subprocess.check_output(["git", "-C", str(root), "rev-parse", expression], text=True).strip()


def write_json_atomic(path: Path, value: dict) -> None:
    fd, temporary = tempfile.mkstemp(prefix=path.name + ".", suffix=".tmp", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as target:
            json.dump(value, target, indent=2, sort_keys=True)
            target.write("\n")
            target.flush()
            os.fsync(target.fileno())
        os.replace(temporary, path)
    except BaseException:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def seal_evidence(directory: Path) -> None:
    lines = []
    for path in sorted(directory.rglob("*")):
        if path.is_file() and path.name != "SHA256SUMS" and not path.is_symlink():
            lines.append(f"{sha256_file(path)}  {path.relative_to(directory).as_posix()}")
    target = directory / "SHA256SUMS"
    with target.open("x", encoding="ascii", newline="\n") as output:
        output.write("\n".join(lines) + "\n")
        output.flush()
        os.fsync(output.fileno())
    os.chmod(target, 0o600)


def child_command(args, iteration: int, output_dir: Path) -> list[str]:
    runner = Path(__file__).with_name("runner.py").resolve()
    prefix = f"{args.run_id}-{iteration:05d}"
    result = output_dir / f"{prefix}.jsonl"
    checkpoint = output_dir / f"{prefix}.sqlite"
    command = [
        sys.executable, str(runner), "--input", str(args.input.resolve()),
        "--manifest", str(args.manifest.resolve()), "--api-url", args.api_url,
        "--run-id", prefix, "--dataset-uri", args.dataset_uri,
        "--dataset-sha256", args.dataset_sha256, "--time-scale", args.time_scale,
        "--submit-rate", str(args.submit_rate), "--concurrency", str(args.concurrency),
        "--max-tasks", str(args.max_tasks), "--max-output-bytes", str(args.max_output_bytes),
        "--max-runtime", "3600", "--token-env", args.token_env,
        "--checkpoint", str(checkpoint), "--output", str(result),
    ]
    for flag, value in (("--ca-file", args.ca_file), ("--source-dataset", args.source_dataset)):
        if value:
            command.extend((flag, str(Path(value).resolve())))
    if args.allow_http_local:
        command.append("--allow-http-local")
    return command


def stop_child(child: subprocess.Popen, timeout: float = 10) -> None:
    if child.poll() is not None:
        return
    child.terminate()
    try:
        child.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        child.kill()
        child.wait(timeout=timeout)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--confirm-run-id", required=True, help="must exactly match --run-id")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--duration", choices=sorted(DURATIONS), required=True)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--source-dataset", type=Path)
    parser.add_argument("--api-url", required=True)
    parser.add_argument("--ca-file")
    parser.add_argument("--dataset-uri", required=True)
    parser.add_argument("--dataset-sha256", required=True)
    parser.add_argument("--time-scale", default="1")
    parser.add_argument("--submit-rate", type=float, default=5)
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--max-tasks", type=int, default=10000)
    parser.add_argument("--max-output-bytes", type=int, default=128 * 1024 * 1024)
    parser.add_argument("--sample-interval-seconds", type=int, default=10)
    parser.add_argument("--min-available-mib", type=int, default=6144)
    parser.add_argument("--max-child-rss-mib", type=int, default=4096)
    parser.add_argument("--max-evidence-bytes", type=int, default=2 * 1024 * 1024 * 1024)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--token-env", default="TASKLAB_BEARER_TOKEN")
    parser.add_argument("--allow-http-local", action="store_true")
    return parser


def validate(args) -> None:
    if args.confirm_run_id != args.run_id or not RUN_ID_RE.fullmatch(args.run_id):
        raise SoakError("run confirmation must exactly match a safe run ID")
    if not args.input.is_file() or not args.manifest.is_file():
        raise SoakError("input and manifest must be readable files")
    if args.source_dataset and not args.source_dataset.is_file():
        raise SoakError("source dataset must be a readable file")
    if not re.fullmatch(r"[0-9a-f]{64}", args.dataset_sha256):
        raise SoakError("dataset SHA256 must be 64 lowercase hex characters")
    if not args.dataset_uri.startswith("artifact://"):
        raise SoakError("dataset URI must use artifact://")
    if not (1 <= args.concurrency <= 64 and 1 <= args.max_tasks <= 100000):
        raise SoakError("concurrency must be 1..64 and max tasks 1..100000")
    if not (0 < args.submit_rate <= 1000 and 0 < args.sample_interval_seconds <= 300):
        raise SoakError("submit rate must be <=1000/s and sample interval 1..300 seconds")
    if args.min_available_mib < 6144:
        raise SoakError("minimum available memory may not be configured below the 6 GiB gate")
    if args.max_child_rss_mib < 1 or args.max_evidence_bytes < 1024:
        raise SoakError("RSS and evidence byte ceilings must be positive")
    token = os.environ.get(args.token_env, "")
    if not token:
        raise SoakError(f"required token environment variable {args.token_env} is empty")
    if args.evidence_dir.exists():
        raise SoakError("evidence directory already exists; refusing to overwrite prior evidence")
    if not args.allow_http_local and not args.api_url.lower().startswith("https://"):
        raise SoakError("HTTPS is required unless loopback HTTP is explicitly enabled")
    if args.allow_http_local and not re.match(r"^http://(localhost|127\.0\.0\.1|\[::1\])(?::|/)", args.api_url.lower()):
        raise SoakError("HTTP is allowed only for loopback endpoints")


def run(args) -> int:
    validate(args)
    if not Path("/proc/meminfo").is_file():
        raise SoakError("Linux /proc resource sampling is required; no replay started")
    root = Path(__file__).resolve().parents[2]
    source_sha = git_value(root, "HEAD")
    source_tree = git_value(root, "HEAD^{tree}")
    if subprocess.check_output(["git", "-C", str(root), "status", "--porcelain"], text=True).strip():
        raise SoakError("source checkout must be clean so the soak manifest binds an exact tree")
    evidence = args.evidence_dir.resolve()
    evidence.mkdir(parents=True, exist_ok=False)
    os.chmod(evidence, 0o700)
    sample_path = evidence / "resource-samples.jsonl"
    runs = []
    started = time.monotonic()
    deadline = started + DURATIONS[args.duration]
    status = "RUNNING"
    reason = None
    child = None
    iteration = 0
    manifest = {
        "mode": "CORE_E2E_SOAK", "status": status, "run_id": args.run_id,
        "source_commit": source_sha, "source_tree": source_tree,
        "input_sha256": sha256_file(args.input), "manifest_sha256": sha256_file(args.manifest),
        "duration_seconds": DURATIONS[args.duration], "started_utc": utc_now(),
        "parameters": {"min_available_mib": args.min_available_mib,
                       "max_child_rss_mib": args.max_child_rss_mib,
                       "max_evidence_bytes": args.max_evidence_bytes,
                       "sample_interval_seconds": args.sample_interval_seconds,
                       "concurrency": args.concurrency, "max_tasks_per_iteration": args.max_tasks},
        "iterations": runs,
    }
    write_json_atomic(evidence / "manifest.json", manifest)
    try:
        with sample_path.open("x", encoding="utf-8", newline="\n") as samples:
            while time.monotonic() < deadline:
                available = linux_memory_mib()
                if available < args.min_available_mib:
                    status, reason = "RESOURCE_LIMITED", f"host available memory {available} MiB below gate"
                    break
                if directory_bytes(evidence) >= args.max_evidence_bytes:
                    status, reason = "RESOURCE_LIMITED", "evidence byte ceiling reached"
                    break
                iteration += 1
                command = child_command(args, iteration, evidence)
                log_path = evidence / f"iteration-{iteration:05d}.log"
                with log_path.open("xb") as log:
                    os.chmod(log_path, 0o600)
                    child = subprocess.Popen(command, stdout=log, stderr=subprocess.STDOUT,
                                             env=os.environ.copy(), cwd=str(root))
                    while child.poll() is None:
                        now = time.monotonic()
                        available = linux_memory_mib()
                        try:
                            child_rss = linux_rss_mib(child.pid)
                        except FileNotFoundError:
                            if child.poll() is not None:
                                break
                            raise
                        sample = {"utc": utc_now(), "iteration": iteration,
                                  "host_available_mib": available,
                                  "child_rss_mib": child_rss}
                        samples.write(json.dumps(sample, sort_keys=True) + "\n")
                        samples.flush()
                        os.fsync(samples.fileno())
                        if sample["host_available_mib"] < args.min_available_mib:
                            status, reason = "RESOURCE_LIMITED", "host available memory crossed the gate"
                            stop_child(child)
                            break
                        if sample["child_rss_mib"] > args.max_child_rss_mib:
                            status, reason = "RESOURCE_LIMITED", "child RSS exceeded the configured ceiling"
                            stop_child(child)
                            break
                        if directory_bytes(evidence) >= args.max_evidence_bytes:
                            status, reason = "RESOURCE_LIMITED", "evidence byte ceiling reached"
                            stop_child(child)
                            break
                        if now >= deadline:
                            status, reason = "INTERRUPTED", "duration ended while an iteration was active"
                            stop_child(child)
                            break
                        time.sleep(min(args.sample_interval_seconds, max(0.1, deadline - now)))
                child_rc = child.poll()
                child = None
                summary_path = (evidence / f"{args.run_id}-{iteration:05d}.jsonl.summary.json")
                iteration_record = {"iteration": iteration, "exit_code": child_rc,
                                    "log": log_path.name,
                                    "summary": summary_path.name if summary_path.is_file() else None}
                if summary_path.is_file():
                    try:
                        iteration_record["status"] = json.loads(summary_path.read_text(encoding="utf-8")).get("status")
                    except (OSError, json.JSONDecodeError):
                        iteration_record["status"] = "INVALID_SUMMARY"
                runs.append(iteration_record)
                manifest["iterations"] = runs
                if status != "RUNNING":
                    break
                if child_rc != 0 or iteration_record.get("status") != "COMPLETED":
                    status, reason = "FAIL", "replay iteration failed or did not report COMPLETED"
                    break
                write_json_atomic(evidence / "manifest.json", manifest)
            if status == "RUNNING":
                status = "COMPLETED"
    except KeyboardInterrupt:
        status, reason = "INTERRUPTED", "operator requested graceful stop"
        if child is not None:
            stop_child(child)
    except Exception as exc:
        status, reason = "FAIL", "runner error: " + type(exc).__name__
        if child is not None:
            stop_child(child)
    finally:
        if child is not None and child.poll() is None:
            stop_child(child)
            if status == "RUNNING":
                status, reason = "INTERRUPTED", "runner stopped while a replay iteration was active"
        manifest.update({"status": status, "reason": reason,
                         "ended_utc": utc_now(), "elapsed_seconds": round(time.monotonic() - started, 3),
                         "iterations": runs})
        write_json_atomic(evidence / "manifest.json", manifest)
        seal_evidence(evidence)
    print(json.dumps({"status": status, "evidence_dir": str(evidence), "iterations": len(runs)}))
    return 0 if status == "COMPLETED" else 3


def main(argv=None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        return run(args)
    except (OSError, SoakError, subprocess.CalledProcessError) as exc:
        print(f"soak runner failed: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
