#!/usr/bin/env python3
"""Bounded Azure LLM workload replay through the TaskLab v1 API.

This is a client-side Core E2E runner, not a scheduler microbenchmark. It only
submits validated eval/gateway tasks through the authenticated API. The caller
must stage the dataset in the configured ArtifactStore and supply its URI/hash.
"""

from __future__ import annotations

import argparse
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
from datetime import datetime
from decimal import Decimal, InvalidOperation
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import ssl
import subprocess
import tempfile
import sys
import threading
import time
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, ProxyHandler, Request, build_opener


TERMINAL = {"success", "failed", "dlq"}
REQUIRED_FIELDS = {
    "schema_version", "workload_id", "source_dataset", "source_record_id",
    "arrival_offset_ms", "queue", "tenant_id", "task_type", "context_tokens",
    "generated_tokens", "duration_model", "expected_outcome", "seed", "provenance",
}
MAX_LINE_BYTES = 1 << 20


class RunnerError(Exception):
    pass


class SubmissionRejected(RunnerError):
    def __init__(self, message: str, retries: int = 0):
        super().__init__(message)
        self.retries = retries


class SubmissionUncertain(RunnerError):
    def __init__(self, message: str, retries: int = 0):
        super().__init__(message)
        self.retries = retries


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def load_and_validate(input_path: Path, manifest_path: Path, source_path: Path | None, max_tasks: int):
    if input_path.stat().st_size > 128 * 1024 * 1024:
        raise RunnerError("input exceeds the 128 MiB runner bound")
    input_hash = sha256_file(input_path)
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RunnerError(f"cannot read replay manifest: {exc}") from exc
    if manifest.get("status") != "GENERATED_NOT_EXECUTED":
        raise RunnerError("manifest status is not GENERATED_NOT_EXECUTED")
    if manifest.get("generated_output_sha256") != input_hash:
        raise RunnerError("input JSONL SHA256 does not match manifest")
    if source_path is not None and sha256_file(source_path) != manifest.get("source_sha256"):
        raise RunnerError("source dataset SHA256 does not match manifest")
    count = 0
    previous = -1
    with input_path.open("rb") as stream:
        for line_no, raw in enumerate(stream, 1):
            if len(raw) > MAX_LINE_BYTES:
                raise RunnerError(f"line {line_no} exceeds {MAX_LINE_BYTES} bytes")
            try:
                item = json.loads(raw)
            except (UnicodeDecodeError, json.JSONDecodeError) as exc:
                raise RunnerError(f"line {line_no}: invalid JSON: {exc}") from exc
            if not isinstance(item, dict) or set(item) != REQUIRED_FIELDS:
                raise RunnerError(f"line {line_no}: schema fields do not match replay schema v1")
            if item["schema_version"] != "1" or item["task_type"] != "eval":
                raise RunnerError(f"line {line_no}: unsupported schema_version or task_type")
            if not isinstance(item["workload_id"], str) or not item["workload_id"] or len(item["workload_id"]) > 256:
                raise RunnerError(f"line {line_no}: invalid workload_id")
            arrival = item["arrival_offset_ms"]
            if isinstance(arrival, bool) or not isinstance(arrival, int) or arrival < previous:
                raise RunnerError(f"line {line_no}: arrival offsets must be nondecreasing non-negative integers")
            previous = arrival
            if not isinstance(item["queue"], str) or not item["queue"].strip() or len(item["queue"]) > 128:
                raise RunnerError(f"line {line_no}: invalid queue")
            if item["duration_model"] != "none" or item["expected_outcome"] not in {"success", "failure"}:
                raise RunnerError(f"line {line_no}: unsupported workload semantics")
            for name in ("context_tokens", "generated_tokens", "seed"):
                if isinstance(item[name], bool) or not isinstance(item[name], int) or item[name] < 0:
                    raise RunnerError(f"line {line_no}: {name} must be a non-negative integer")
            if not isinstance(item["tenant_id"], str) or not isinstance(item["provenance"], dict):
                raise RunnerError(f"line {line_no}: invalid tenant/provenance metadata")
            count += 1
            if count > max_tasks:
                raise RunnerError(f"input exceeds --max-tasks ({max_tasks})")
    if count == 0:
        raise RunnerError("input JSONL contains no records")
    if count != manifest.get("generated_records"):
        raise RunnerError("input record count does not match manifest")
    return input_hash, count, previous, manifest


def iter_records(path: Path):
    with path.open("r", encoding="utf-8") as stream:
        for ordinal, line in enumerate(stream, 1):
            yield ordinal, json.loads(line)


def api_request(base_url: str, token: str, method: str, path: str, payload: dict | None,
                timeout: float, ca_file: str | None, allow_http_local: bool, extra_headers: dict | None = None):
    url = base_url.rstrip("/") + "/v1" + path
    headers = {"Accept": "application/json", "Authorization": "Bearer " + token}
    if extra_headers:
        headers.update(extra_headers)
    data = None
    if payload is not None:
        data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = Request(url, data=data, headers=headers, method=method)
    context = ssl.create_default_context(cafile=ca_file) if ca_file else ssl.create_default_context()
    opener = build_opener(NoRedirect, ProxyHandler({}), HTTPSHandler(context=context))
    try:
        with opener.open(request, timeout=timeout) as response:
            raw = response.read((1 << 20) + 1)
            if len(raw) > 1 << 20:
                raise RunnerError("API response exceeds 1 MiB")
            return response.status, json.loads(raw or b"{}"), response.headers
    except HTTPError as exc:
        raw = exc.read((1 << 20) + 1)
        if len(raw) > 1 << 20:
            raise RunnerError("API error response exceeds 1 MiB")
        try:
            body = json.loads(raw or b"{}")
        except json.JSONDecodeError:
            body = {}
        return exc.code, body, exc.headers
    except (URLError, TimeoutError, OSError) as exc:
        raise RunnerError(f"API transport error: {type(exc).__name__}") from exc


def parse_timestamp(value: str | None):
    if not value:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        return parsed.timestamp()
    except (ValueError, TypeError):
        return None


def create_and_wait(record: dict, ordinal: int, args, input_hash: str, token: str, cancel: threading.Event):
    idempotency = hashlib.sha256(f"{args.run_id}\0{input_hash}\0{ordinal}\0{record['workload_id']}".encode()).hexdigest()
    task_id = None
    submit_started = time.monotonic()
    wall_submit_started = time.time()
    payload = {
        "type": "eval",
        "queue": record["queue"],
        "max_attempts": 1,
        "timeout_seconds": args.task_timeout,
        "retry_delay_seconds": 1,
        "retry_backoff_multiplier": 2,
        "retry_jitter": False,
        "idempotency_key": idempotency,
        "payload": {"version": "1", "kind": "eval", "data": {
            "execution_mode": "gateway", "version": "2",
            "dataset_version": args.dataset_version, "dataset_uri": args.dataset_uri,
            "dataset_sha256": args.dataset_sha256, "model_profile": args.model_profile,
            "prompt_version": args.prompt_version, "retriever_version": args.retriever_version,
            "evaluator_schema_version": args.evaluator_schema_version,
            "run_id": f"{args.run_id}-{ordinal}", "case_start": ordinal - 1, "case_end": ordinal,
        }},
    }
    last_error = None
    retry_count = 0
    for attempt in range(args.max_retries + 1):
        if cancel.is_set():
            raise RunnerError("cancelled")
        try:
            status, response, headers = api_request(args.api_url, token, "POST", "/tasks", payload,
                                                   args.request_timeout, args.ca_file, args.allow_http_local,
                                                   {"Idempotency-Key": idempotency})
            if status == 201 or (status == 200 and response.get("id")):
                task_id = response.get("id")
                if not task_id:
                    raise RunnerError("API accepted request without a task id")
                break
            if status in (429, 500, 502, 503, 504) and attempt < args.max_retries:
                retry_count += 1
                retry_after = headers.get("Retry-After", "")
                delay = min(float(retry_after) if retry_after.isdecimal() else 0.25 * (2 ** attempt), args.retry_max_delay)
                if cancel.wait(delay):
                    raise RunnerError("cancelled")
                continue
            error = response.get("error", {}) if isinstance(response, dict) else {}
            raise SubmissionRejected(f"API rejected task: HTTP {status} {error.get('code', 'unknown')}", retry_count)
        except RunnerError as exc:
            last_error = exc
            # A transport failure may follow a committed enqueue. Retry only
            # with the exact same idempotency key and request body.
            if "transport error" not in str(exc):
                raise
            if attempt >= args.max_retries:
                raise SubmissionUncertain("submission outcome uncertain after bounded transport retries", retry_count) from exc
            retry_count += 1
            if cancel.wait(min(0.25 * (2 ** attempt), args.retry_max_delay)):
                raise RunnerError("cancelled")
    if task_id is None:
        raise last_error or RunnerError("task submission failed")
    accepted_at = time.monotonic()
    accepted_wall = time.time()
    deadline = accepted_at + args.task_deadline
    api_latency = (accepted_at - submit_started) * 1000
    while time.monotonic() < deadline:
        if cancel.is_set():
            raise RunnerError("cancelled")
        try:
            status, task, _ = api_request(args.api_url, token, "GET", "/tasks/" + quote(str(task_id), safe=""),
                                          None, args.request_timeout, args.ca_file, args.allow_http_local)
            if status == 200:
                state = task.get("status")
                if state in TERMINAL:
                    created = parse_timestamp(task.get("created_at"))
                    started = parse_timestamp(task.get("started_at"))
                    finished = parse_timestamp(task.get("finished_at"))
                    return {"ordinal": ordinal, "workload_id": record["workload_id"], "task_id": task_id,
                            "idempotency_key": idempotency, "status": state, "http_submit_ms": api_latency,
                            "client_retries": retry_count,
                            "accepted_at_monotonic": accepted_at, "terminal_observed_at_monotonic": time.monotonic(),
                            "attempt": task.get("attempt"), "lease_version": task.get("lease_version"),
                            "task_result": task.get("result"),
                            "task_created_at": task.get("created_at"), "task_started_at": task.get("started_at"),
                            "task_finished_at": task.get("finished_at"),
                            "queue_wait_ms": (started-created)*1000 if created and started else None,
                            "e2e_latency_ms": (finished-created)*1000 if created and finished else None,
                            "tenant_metadata_only": record["tenant_id"]}
            elif status in (429, 500, 502, 503, 504):
                pass
            elif status != 404:
                return {"ordinal": ordinal, "workload_id": record["workload_id"], "task_id": task_id,
                        "idempotency_key": idempotency, "status": "poll_error", "error": f"task polling failed: HTTP {status}",
                        "http_submit_ms": api_latency, "client_retries": retry_count,
                        "accepted_at_monotonic": accepted_at, "terminal_observed_at_monotonic": None,
                        "attempt": None, "lease_version": None, "task_result": None,
                        "task_created_at": None, "task_started_at": None, "task_finished_at": None,
                        "queue_wait_ms": None, "e2e_latency_ms": None, "tenant_metadata_only": record["tenant_id"]}
        except RunnerError as exc:
            if "transport error" not in str(exc):
                raise
        cancel.wait(args.poll_interval)
    return {"ordinal": ordinal, "workload_id": record["workload_id"], "task_id": task_id,
            "idempotency_key": idempotency, "status": "unfinished", "http_submit_ms": api_latency,
            "client_retries": retry_count,
            "accepted_at_monotonic": accepted_at, "terminal_observed_at_monotonic": None,
            "attempt": None, "lease_version": None, "task_result": None,
            "task_created_at": None, "task_started_at": None, "task_finished_at": None,
            "queue_wait_ms": None, "e2e_latency_ms": None, "tenant_metadata_only": record["tenant_id"]}


def initialize_checkpoint(path: Path, input_hash: str, run_id: str, config_hash: str):
    path.parent.mkdir(parents=True, exist_ok=True)
    db = sqlite3.connect(path)
    db.execute("PRAGMA journal_mode=WAL")
    db.execute("CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
    db.execute("CREATE TABLE IF NOT EXISTS results (ordinal INTEGER PRIMARY KEY, status TEXT NOT NULL, json TEXT NOT NULL, http_submit_ms REAL, queue_wait_ms REAL, e2e_latency_ms REAL)")
    db.execute("CREATE TABLE IF NOT EXISTS attempts (attempt_id INTEGER PRIMARY KEY AUTOINCREMENT, ordinal INTEGER NOT NULL, status TEXT NOT NULL, json TEXT NOT NULL)")
    existing = dict(db.execute("SELECT key,value FROM metadata"))
    expected = {"input_sha256": input_hash, "run_id": run_id, "config_sha256": config_hash}
    if existing and any(existing.get(key) != value for key, value in expected.items()):
        db.close()
        raise RunnerError("checkpoint belongs to a different input hash or run id")
    db.executemany("INSERT OR IGNORE INTO metadata(key,value) VALUES(?,?)", expected.items())
    db.commit()
    return db


def percentile(db, field: str, fraction: float):
    count = db.execute(f"SELECT COUNT({field}) FROM results WHERE {field} IS NOT NULL").fetchone()[0]
    if not count:
        return None
    offset = max(0, int((count - 1) * fraction))
    value = db.execute(f"SELECT {field} FROM results WHERE {field} IS NOT NULL ORDER BY {field} LIMIT 1 OFFSET ?", (offset,)).fetchone()[0]
    return round(value, 3)


def git_evidence():
    root = Path(__file__).resolve().parents[2]
    def git(*args):
        try:
            result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True, timeout=5, check=True)
            return result.stdout.strip()
        except (OSError, subprocess.SubprocessError):
            return "UNAVAILABLE"
    dirty = git("status", "--porcelain")
    diff = git("diff", "--binary", "HEAD")
    return {"source_commit": git("rev-parse", "HEAD"), "source_tree": git("rev-parse", "HEAD^{tree}"),
            "source_worktree_dirty": dirty not in ("", "UNAVAILABLE"),
            "tracked_worktree_diff_sha256": hashlib.sha256(diff.encode("utf-8")).hexdigest() if diff != "UNAVAILABLE" else diff,
            "runner_sha256": sha256_file(Path(__file__)),
            "runner_tests_sha256": sha256_file(Path(__file__).with_name("test_runner.py")),
            "generator_sha256": sha256_file(Path(__file__).with_name("azure_llm.py"))}


def run(args):
    input_path = args.input.resolve(strict=True)
    manifest_path = args.manifest.resolve(strict=True)
    input_hash, planned, max_arrival_ms, manifest = load_and_validate(input_path, manifest_path, args.source_dataset, args.max_tasks)
    if max_arrival_ms / float(args.time_scale) / 1000.0 > args.max_runtime:
        raise RunnerError("scaled workload arrival span exceeds --max-runtime")
    if not args.dataset_uri.startswith("artifact://") or len(args.dataset_sha256) != 64 or any(ch not in "0123456789abcdefABCDEF" for ch in args.dataset_sha256):
        raise RunnerError("dataset URI must be artifact:// and dataset SHA256 must be 64 hex characters")
    parts = urlsplit(args.api_url)
    if parts.scheme not in {"https", "http"} or not parts.hostname or parts.username or parts.password or parts.query or parts.fragment:
        raise RunnerError("API URL must be an absolute HTTP(S) URL without credentials/query/fragment")
    if parts.scheme == "http" and not (args.allow_http_local and parts.hostname in {"127.0.0.1", "localhost", "::1"}):
        raise RunnerError("HTTP is allowed only for loopback with --allow-http-local")
    result_path = args.output.resolve()
    summary_path = result_path.with_suffix(result_path.suffix + ".summary.json")
    forbidden = {input_path, manifest_path, args.checkpoint.resolve()}
    if args.source_dataset is not None:
        forbidden.add(args.source_dataset.resolve())
    if result_path in forbidden or summary_path in forbidden:
        raise RunnerError("input, manifest, source, checkpoint, and output paths must be distinct")
    checkpoint_exists = args.checkpoint.exists()
    if (result_path.exists() or summary_path.exists()) and not (args.resume and checkpoint_exists):
        raise RunnerError("refusing to overwrite existing result evidence; use a new path or explicit --resume")
    if args.resume and not checkpoint_exists:
        raise RunnerError("--resume requires an existing checkpoint")
    token = os.environ.get(args.token_env, "").strip()
    if not token:
        raise RunnerError(f"required bearer token environment variable {args.token_env} is empty")
    config_hash = hashlib.sha256(json.dumps({"api_url": args.api_url, "dataset_uri": args.dataset_uri,
        "dataset_sha256": args.dataset_sha256, "dataset_version": args.dataset_version,
        "model_profile": args.model_profile, "prompt_version": args.prompt_version,
        "retriever_version": args.retriever_version, "evaluator_schema_version": args.evaluator_schema_version,
    }, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    db = initialize_checkpoint(args.checkpoint, input_hash, args.run_id, config_hash)
    # Bounded concurrency; SQLite and evidence are updated only by this thread.
    inflight = {}
    cancel = threading.Event()
    started = time.monotonic()
    submit_started = time.monotonic()
    last_accepted_monotonic = None
    last_terminal_monotonic = None
    errors = 0
    retries = 0
    next_submit = time.monotonic()
    executor = ThreadPoolExecutor(max_workers=args.concurrency, thread_name_prefix="replay")

    def consume_one():
        nonlocal errors, retries, last_accepted_monotonic, last_terminal_monotonic
        done, _ = wait(inflight, return_when=FIRST_COMPLETED)
        for future in done:
            ordinal = inflight.pop(future)
            try:
                result = future.result()
            except SubmissionRejected as exc:
                errors += 1
                result = {"ordinal": ordinal, "status": "submission_rejected", "error": str(exc)[:256],
                          "client_retries": exc.retries, "task_id": None, "http_submit_ms": None, "queue_wait_ms": None, "e2e_latency_ms": None}
            except SubmissionUncertain as exc:
                errors += 1
                result = {"ordinal": ordinal, "status": "submission_uncertain", "error": str(exc)[:256],
                          "client_retries": exc.retries, "task_id": None, "http_submit_ms": None, "queue_wait_ms": None, "e2e_latency_ms": None}
            except Exception as exc:
                errors += 1
                result = {"ordinal": ordinal, "status": "runner_error", "error": str(exc)[:256],
                          "task_id": None, "http_submit_ms": None, "queue_wait_ms": None, "e2e_latency_ms": None}
            if result.get("accepted_at_monotonic") is not None:
                last_accepted_monotonic = max(last_accepted_monotonic or 0, result["accepted_at_monotonic"])
            if result.get("terminal_observed_at_monotonic") is not None:
                last_terminal_monotonic = max(last_terminal_monotonic or 0, result["terminal_observed_at_monotonic"])
            retries += int(result.get("client_retries", 0))
            result.pop("accepted_at_monotonic", None)
            result.pop("terminal_observed_at_monotonic", None)
            encoded = json.dumps(result, sort_keys=True)
            db.execute("INSERT INTO attempts(ordinal,status,json) VALUES(?,?,?)", (ordinal, result["status"], encoded))
            db.execute("INSERT OR REPLACE INTO results(ordinal,status,json,http_submit_ms,queue_wait_ms,e2e_latency_ms) VALUES(?,?,?,?,?,?)",
                       (ordinal, result["status"], encoded, result.get("http_submit_ms"), result.get("queue_wait_ms"), result.get("e2e_latency_ms")))
            db.commit()

    try:
        for ordinal, record in iter_records(input_path):
            prior = db.execute("SELECT status FROM results WHERE ordinal=?", (ordinal,)).fetchone()
            if prior and prior[0] not in {"submission_uncertain", "poll_error"}:
                continue
            target = started + (record["arrival_offset_ms"] / 1000.0) / float(args.time_scale)
            if time.monotonic() - started > args.max_runtime:
                break
            while True:
                delay = target - time.monotonic()
                if delay <= 0:
                    break
                if cancel.wait(min(delay, 0.2)):
                    raise KeyboardInterrupt
            now = time.monotonic()
            next_submit = max(next_submit, now)
            if now < next_submit:
                cancel.wait(next_submit - now)
            next_submit = max(next_submit, time.monotonic()) + 1.0 / float(args.submit_rate)
            if len(inflight) >= args.concurrency:
                consume_one()
            if time.monotonic() - started > args.max_runtime:
                break
            inflight[executor.submit(create_and_wait, record, ordinal, args, input_hash, token, cancel)] = ordinal
        while inflight:
            consume_one()
    except KeyboardInterrupt:
        cancel.set()
        executor.shutdown(wait=False)
        db.commit()
        db.close()
        raise
    finally:
        cancel.set()
        executor.shutdown(wait=True)

    duration = time.monotonic() - started
    counts = dict(db.execute("SELECT status,COUNT(*) FROM results GROUP BY status"))
    accepted = sum(counts.get(status, 0) for status in ("success", "failed", "dlq", "unfinished", "poll_error"))
    terminal = sum(counts.get(status, 0) for status in TERMINAL)
    all_done = db.execute("SELECT COUNT(*) FROM results").fetchone()[0]
    result_bytes = db.execute("SELECT COALESCE(SUM(length(json) + 1), 0) FROM results").fetchone()[0]
    if result_bytes > args.max_output_bytes:
        db.close()
        raise RunnerError(f"result evidence exceeds --max-output-bytes ({args.max_output_bytes})")
    submit_duration = max(0.0, last_accepted_monotonic - submit_started) if last_accepted_monotonic else 0.0
    drain_duration = max(0.0, last_terminal_monotonic - last_accepted_monotonic) if last_terminal_monotonic and last_accepted_monotonic else None
    result_path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=result_path.name + ".", suffix=".tmp", dir=result_path.parent)
    tmp = Path(tmp_name)
    with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as output:
        for (line,) in db.execute("SELECT json FROM results ORDER BY ordinal"):
            output.write(line + "\n")
        output.flush()
        os.fsync(output.fileno())
    if args.resume:
        os.replace(tmp, result_path)
    else:
        os.link(tmp, result_path)
        tmp.unlink()
    enqueue_percentiles = {f"p{int(p*100)}": percentile(db, "http_submit_ms", p) for p in (.50, .95, .99)}
    queue_percentiles = {f"p{int(p*100)}": percentile(db, "queue_wait_ms", p) for p in (.50, .95, .99)}
    e2e_percentiles = {f"p{int(p*100)}": percentile(db, "e2e_latency_ms", p) for p in (.50, .95, .99)}
    db.close()
    summary = {
        "mode": "CORE_E2E_BENCHMARK", "status": "COMPLETED" if all_done == planned and counts.get("unfinished", 0) == 0 and counts.get("submission_rejected", 0) == 0 and counts.get("submission_uncertain", 0) == 0 and counts.get("poll_error", 0) == 0 and counts.get("runner_error", 0) == 0 else "INCOMPLETE",
        **git_evidence(),
        "run_id": args.run_id, "input_sha256": input_hash, "source_sha256": manifest.get("source_sha256"),
        "planned_tasks": planned, "submitted_requests": all_done, "accepted_tasks": accepted,
        "rejected_requests": counts.get("submission_rejected", 0), "terminal_success": counts.get("success", 0),
        "terminal_failed": counts.get("failed", 0), "terminal_dlq": counts.get("dlq", 0),
        "unfinished_tasks": counts.get("unfinished", 0), "not_submitted_tasks": max(0, planned - all_done),
        "submission_uncertain": counts.get("submission_uncertain", 0), "poll_errors": counts.get("poll_error", 0),
        "runner_errors": counts.get("runner_error", 0), "submission_errors": counts.get("submission_rejected", 0) + counts.get("submission_uncertain", 0),
        "client_retries": retries, "duplicate_logical_tasks": "UNAVAILABLE",
        "test_duration_seconds": round(duration, 3), "submit_duration_seconds": round(submit_duration, 3),
        "drain_duration_seconds": round(drain_duration, 3) if drain_duration is not None else "UNAVAILABLE", "completed_throughput_tasks_per_second": round(terminal / duration, 3) if duration > 0 else None,
        "enqueue_latency_ms": enqueue_percentiles,
        "queue_wait_latency_ms": queue_percentiles,
        "e2e_latency_ms": e2e_percentiles,
        "notes": ["single authenticated principal; tenant_id is source metadata only", "synthetic expected outcomes are not used to rewrite results", "the API v1 response does not disclose whether an idempotency key created or returned an existing logical task"],
        "results_jsonl": str(result_path), "checkpoint_sqlite": str(args.checkpoint.resolve()),
        "results_sha256": sha256_file(result_path), "checkpoint_sha256": sha256_file(args.checkpoint),
    }
    output_summary = summary_path
    fd, summary_tmp_name = tempfile.mkstemp(prefix=output_summary.name + ".", suffix=".tmp", dir=output_summary.parent)
    summary_tmp = Path(summary_tmp_name)
    with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as stream:
        json.dump(summary, stream, sort_keys=True, indent=2)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    if args.resume:
        os.replace(summary_tmp, output_summary)
    else:
        os.link(summary_tmp, output_summary)
        summary_tmp.unlink()
    print(json.dumps(summary, sort_keys=True))
    return 0 if summary["status"] == "COMPLETED" else 3


def decimal_gt_zero(value: str):
    try:
        parsed = Decimal(value)
    except InvalidOperation as exc:
        raise argparse.ArgumentTypeError("must be a finite decimal > 0") from exc
    if not parsed.is_finite() or parsed <= 0:
        raise argparse.ArgumentTypeError("must be a finite decimal > 0")
    return parsed


def build_parser():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--source-dataset", type=Path)
    parser.add_argument("--api-url", required=True)
    parser.add_argument("--token-env", default="TASKLAB_BEARER_TOKEN", help="environment variable name; token is never accepted on argv")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--dataset-uri", required=True, help="pre-staged artifact:// URI")
    parser.add_argument("--dataset-sha256", required=True, help="hash of the pre-staged dataset object")
    parser.add_argument("--dataset-version", default="azure-llm-2023-v1")
    parser.add_argument("--model-profile", default="staging-sim")
    parser.add_argument("--prompt-version", default="replay-v1")
    parser.add_argument("--retriever-version", default="staging-sim-v1")
    parser.add_argument("--evaluator-schema-version", default="v1")
    parser.add_argument("--time-scale", type=decimal_gt_zero, default=Decimal("1"))
    parser.add_argument("--submit-rate", type=decimal_gt_zero, default=Decimal("10"), help="maximum task submissions per second")
    parser.add_argument("--concurrency", type=int, default=4, help="maximum in-flight tasks, 1..64")
    parser.add_argument("--max-tasks", type=int, default=10000)
    parser.add_argument("--max-output-bytes", type=int, default=128 * 1024 * 1024)
    parser.add_argument("--request-timeout", type=float, default=10)
    parser.add_argument("--task-deadline", type=float, default=300)
    parser.add_argument("--max-runtime", type=float, default=3600, help="maximum scaled arrival span; task drain is separately bounded")
    parser.add_argument("--task-timeout", type=int, default=120)
    parser.add_argument("--poll-interval", type=float, default=1)
    parser.add_argument("--max-retries", type=int, default=3)
    parser.add_argument("--retry-max-delay", type=float, default=5)
    parser.add_argument("--ca-file")
    parser.add_argument("--allow-http-local", action="store_true")
    parser.add_argument("--checkpoint", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--resume", action="store_true", help="resume only when checkpoint and run identity match")
    return parser


def main(argv=None):
    parser = build_parser()
    args = parser.parse_args(argv)
    if not 1 <= args.concurrency <= 64 or not 1 <= args.max_tasks <= 100000 or args.max_retries < 0 or args.max_output_bytes < 1:
        parser.error("concurrency must be 1..64, max-tasks 1..100000, max-retries non-negative")
    if not 0 < float(args.submit_rate) <= 1000 or args.max_runtime > 86400:
        parser.error("submit-rate must be <=1000/s and max-runtime <=24h")
    if min(args.request_timeout, args.task_deadline, args.max_runtime, args.task_timeout, args.poll_interval, args.retry_max_delay) <= 0:
        parser.error("timeouts and retry delay must be positive")
    try:
        return run(args)
    except (OSError, RunnerError, sqlite3.Error) as exc:
        print(f"replay runner failed: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
