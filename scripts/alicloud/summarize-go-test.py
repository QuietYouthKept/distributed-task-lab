#!/usr/bin/env python3
"""Summarize go test -json events and enforce a complete, non-skipped suite."""

from __future__ import annotations

import argparse
import json
import pathlib
import sys


def summarize(source: pathlib.Path, required_tests: set[str]) -> dict:
    counts = {"run": 0, "pass": 0, "fail": 0, "skip": 0}
    packages = {}
    executed_tests = set()
    for line in source.read_text(encoding="utf-8").splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        action = event.get("Action")
        package = event.get("Package", "")
        test = event.get("Test", "")
        if test and action in counts:
            counts[action] += 1
        if test and action == "run":
            executed_tests.add(test)
        if package and action in ("pass", "fail") and not test:
            packages[package] = action

    missing_tests = sorted(required_tests - executed_tests)
    result = {
        "tests": counts,
        "packages": packages,
        "required_tests": sorted(required_tests),
        "missing_required_tests": missing_tests,
        "pass_claimed": bool(
            counts["run"]
            and not counts["fail"]
            and not counts["skip"]
            and packages
            and all(value == "pass" for value in packages.values())
            and not missing_tests
        ),
    }
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=pathlib.Path, help="go test -json event stream")
    parser.add_argument("destination", type=pathlib.Path, help="summary JSON output")
    parser.add_argument(
        "--require-test",
        action="append",
        default=[],
        help="exact Go test name that must have a run event; may be repeated",
    )
    args = parser.parse_args()

    result = summarize(args.source, set(args.require_test))
    args.destination.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(result, sort_keys=True))
    return 0 if result["pass_claimed"] else 1


if __name__ == "__main__":
    sys.exit(main())
