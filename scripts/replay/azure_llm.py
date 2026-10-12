#!/usr/bin/env python3
"""Stream Azure LLM inference trace CSV into deterministic workload JSONL.

The output is a replay manifest format, not an API request body. A runner must
still pass tasks through the project's normal auth, admission and validation
boundaries. Only the source token counts are observed; replay metadata and
expected outcomes are explicitly marked synthetic or derived.
"""

from __future__ import annotations

import argparse
import csv
from datetime import datetime
from decimal import Decimal, InvalidOperation
import hashlib
import json
import math
import os
from pathlib import Path
import random
import re
import sys
import tempfile
from typing import List


DATASET_NAME = "azure-llm-2023"
HEADERS = ["TIMESTAMP", "ContextTokens", "GeneratedTokens"]
TIMESTAMP_RE = re.compile(r"^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?$")
MAX_TOKEN = 2**31 - 1


class ReplayError(ValueError):
    pass


def _timestamp_ns(value: str, line: int) -> int:
    match = TIMESTAMP_RE.fullmatch(value.strip())
    if not match:
        raise ReplayError(f"line {line}: timestamp must be YYYY-MM-DD HH:MM:SS[.fraction]")
    try:
        whole = datetime.strptime(match.group(1), "%Y-%m-%d %H:%M:%S")
    except ValueError as exc:
        raise ReplayError(f"line {line}: invalid calendar timestamp") from exc
    fraction = (match.group(2) or "").ljust(9, "0")
    seconds = whole.toordinal() * 86400 + whole.hour * 3600 + whole.minute * 60 + whole.second
    return seconds * 1_000_000_000 + int(fraction or "0")


def _read_records(path: Path):
    with path.open("r", encoding="utf-8-sig", newline="") as stream:
        reader = csv.DictReader(stream, strict=True)
        if reader.fieldnames != HEADERS:
            raise ReplayError(f"unexpected CSV header: {reader.fieldnames!r}")
        previous_ns = None
        for line, row in enumerate(reader, 2):
            if None in row or any(row.get(name) is None for name in HEADERS):
                raise ReplayError(f"line {line}: malformed CSV field count")
            timestamp = (row["TIMESTAMP"] or "").strip()
            context = (row["ContextTokens"] or "").strip()
            generated = (row["GeneratedTokens"] or "").strip()
            if not context.isascii() or not context.isdecimal() or not generated.isascii() or not generated.isdecimal():
                raise ReplayError(f"line {line}: token counts must be non-negative decimal integers")
            context_tokens, generated_tokens = int(context), int(generated)
            if context_tokens > MAX_TOKEN or generated_tokens > MAX_TOKEN:
                raise ReplayError(f"line {line}: token count exceeds supported signed 32-bit range")
            timestamp_ns = _timestamp_ns(timestamp, line)
            if previous_ns is not None and timestamp_ns < previous_ns:
                raise ReplayError(f"line {line}: timestamps are not nondecreasing")
            previous_ns = timestamp_ns
            yield {
                "timestamp": timestamp,
                "timestamp_ns": timestamp_ns,
                "context_tokens": context_tokens,
                "generated_tokens": generated_tokens,
            }


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _fraction(value: str) -> Decimal:
    try:
        result = Decimal(value)
    except InvalidOperation as exc:
        raise argparse.ArgumentTypeError("time scale must be a finite decimal greater than zero") from exc
    if not result.is_finite() or result <= 0:
        raise argparse.ArgumentTypeError("time scale must be a finite decimal greater than zero")
    return result


def generate(args: argparse.Namespace) -> dict:
    source = args.input.resolve(strict=True)
    output = args.output.resolve()
    manifest_path = output.with_suffix(output.suffix + ".manifest.json")
    if source == output or source == manifest_path:
        raise ReplayError("output must not overwrite the source dataset")
    if output.exists() or manifest_path.exists():
        raise ReplayError("refusing to overwrite an existing output or manifest")
    if args.count <= 0 or args.count > args.max_records:
        raise ReplayError(f"count must be between 1 and --max-records ({args.max_records})")
    queues = [value.strip() for value in args.queues.split(",") if value.strip()]
    if not queues or len(set(queues)) != len(queues):
        raise ReplayError("queues must be a comma-separated list of unique, non-empty names")
    if len(queues) > 64 or len(queues) > args.tenants * 16:
        raise ReplayError("queue/tenant fanout exceeds the configured safe bound")

    source_hash = _sha256_file(source)
    # First bounded streaming pass validates the full input and establishes the
    # source span/count. No request rows are retained in memory.
    source_count = 0
    first_ns = last_ns = None
    for record in _read_records(source):
        if first_ns is None:
            first_ns = record["timestamp_ns"]
        last_ns = record["timestamp_ns"]
        source_count += 1
    if source_count == 0:
        raise ReplayError("source contains no records")

    seed = args.seed
    rng = random.Random(seed)
    time_scale = args.time_scale
    scale_numerator, scale_denominator = time_scale.as_integer_ratio()
    span_ns = last_ns - first_ns
    # One millisecond separates repeated source passes even for single-row data.
    cycle_ns = span_ns + 1_000_000
    output.parent.mkdir(parents=True, exist_ok=True)
    fd, temp_name = tempfile.mkstemp(prefix=f".{output.name}.", suffix=".tmp", dir=output.parent)
    tmp_output = Path(temp_name)
    output_hash = hashlib.sha256()
    source_iter = iter(_read_records(source))
    generated = 0
    max_arrival_ms = 0
    try:
        with os.fdopen(fd, "wb") as target:
            for ordinal in range(args.count):
                try:
                    record = next(source_iter)
                except StopIteration:
                    source_iter = iter(_read_records(source))
                    record = next(source_iter)
                cycle = ordinal // source_count
                offset_ns = (record["timestamp_ns"] - first_ns) + cycle * cycle_ns
                # arrival_offset_ms is integer by schema; half-up rounding avoids
                # Python's platform-independent but surprising bankers rounding.
                scaled_ns = (offset_ns * scale_denominator + scale_numerator // 2) // scale_numerator
                arrival_ms = (scaled_ns + 500_000) // 1_000_000
                max_arrival_ms = max(max_arrival_ms, arrival_ms)
                queue = queues[rng.randrange(len(queues))]
                tenant = 1 + rng.randrange(args.tenants)
                workload = "code" if "code" in source.name.lower() else "conversation"
                item = {
                    "schema_version": "1",
                    "workload_id": f"azure-llm-2023-{workload}-{ordinal + 1:09d}",
                    "source_dataset": DATASET_NAME,
                    "source_record_id": f"row-{ordinal % source_count + 1:09d}",
                    "arrival_offset_ms": arrival_ms,
                    "queue": queue,
                    "tenant_id": f"synthetic-tenant-{tenant:04d}",
                    "task_type": "eval",
                    "context_tokens": record["context_tokens"],
                    "generated_tokens": record["generated_tokens"],
                    "duration_model": "none",
                    "expected_outcome": "success",
                    "seed": seed,
                    "provenance": {
                        "source_record_id": "DERIVED",
                        "arrival_offset_ms": "DERIVED",
                        "context_tokens": "OBSERVED",
                        "generated_tokens": "OBSERVED",
                        "queue": "SYNTHETIC",
                        "tenant_id": "SYNTHETIC",
                        "expected_outcome": "SYNTHETIC",
                        "seed": "SYNTHETIC",
                    },
                }
                encoded = (json.dumps(item, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode()
                target.write(encoded)
                output_hash.update(encoded)
                generated += 1
                if target.tell() > args.max_output_bytes:
                    raise ReplayError(f"generated output exceeds --max-output-bytes ({args.max_output_bytes})")
            target.flush()
            os.fsync(target.fileno())

        if generated != args.count:
            raise ReplayError(f"internal record count mismatch: {generated} != {args.count}")
        if _sha256_file(source) != source_hash:
            raise ReplayError("source changed while generating replay; refusing to publish output")
        # Linking a completed temp file into place is atomic and fails rather
        # than replacing a file that appeared concurrently.
        os.link(tmp_output, output)
        tmp_output.unlink()
        manifest = {
            "schema_version": 1,
            "status": "GENERATED_NOT_EXECUTED",
            "source_dataset": DATASET_NAME,
            "source_file_name": source.name,
            "source_sha256": source_hash,
            "source_records": source_count,
            "generated_records": generated,
            "repeated_source_passes": math.ceil(generated / source_count),
            "time_scale": str(time_scale),
            "seed": seed,
            "queues": queues,
            "tenants": args.tenants,
            "time_zone": "not encoded; original naive timestamp semantics retained",
            "duration_semantics": "none; source does not provide service execution durations",
            "arrival_offset_semantics": "DERIVED from observed timestamps, scaled, rounded to nearest millisecond; repeated passes are SYNTHETIC",
            "max_arrival_offset_ms": max_arrival_ms,
            "generated_output_sha256": output_hash.hexdigest(),
            "generated_output_bytes": output.stat().st_size,
            "output": str(output),
            "warning": "workload records are not API requests and must not bypass normal authentication, admission, payload validation or execution policy",
        }
        with manifest_path.open("x", encoding="utf-8", newline="\n") as stream:
            json.dump(manifest, stream, sort_keys=True, indent=2)
            stream.write("\n")
        return manifest
    except Exception:
        tmp_output.unlink(missing_ok=True)
        # If output was atomically published but manifest creation failed, retain
        # the output as evidence rather than silently deleting it.
        raise


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True, help="Azure LLM Code or Conversation CSV")
    parser.add_argument("--output", type=Path, required=True, help="new JSONL output path (must not exist)")
    parser.add_argument("--count", type=int, required=True, help="requested replay records")
    parser.add_argument("--seed", type=int, default=12345)
    parser.add_argument("--time-scale", type=_fraction, default=Decimal("1"), help="1 preserves span; 10 compresses it by 10x")
    parser.add_argument("--queues", default="llm", help="comma-separated queue labels used in output metadata")
    parser.add_argument("--tenants", type=int, default=1)
    parser.add_argument("--max-records", type=int, default=1_000_000)
    parser.add_argument("--max-output-bytes", type=int, default=128 * 1024 * 1024)
    return parser


def main(argv: List[str] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.tenants < 1 or args.tenants > 1024:
        parser.error("--tenants must be between 1 and 1024")
    if args.max_records < 1 or args.max_output_bytes < 1:
        parser.error("output bounds must be positive")
    try:
        manifest = generate(args)
    except (OSError, csv.Error, ReplayError) as exc:
        print(f"replay generation failed: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(manifest, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
