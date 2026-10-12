#!/usr/bin/env python3
"""Redact exact run secrets and common bearer/password forms from log input."""

import argparse
import pathlib
import re
import sys


parser = argparse.ArgumentParser()
parser.add_argument("--secret-dir", required=True)
args = parser.parse_args()
secret_root = pathlib.Path(args.secret_dir).resolve(strict=True)
secrets = []
for path in secret_root.rglob("*"):
    if path.is_file():
        try:
            value = path.read_text(encoding="utf-8").strip()
        except (UnicodeDecodeError, OSError):
            continue
        if len(value) >= 4:
            secrets.append(value)

content = sys.stdin.read()
for value in sorted(set(secrets), key=len, reverse=True):
    content = content.replace(value, "[REDACTED]")
content = re.sub(r"(?i)(authorization\s*:\s*bearer\s+)[^\s,;]+", r"\1[REDACTED]", content)
content = re.sub(
    r"(?i)((?:password|secret|token|api[_-]?key|access[_-]?key[_-]?secret)\s*[=:]\s*)[^\s,;]+",
    r"\1[REDACTED]",
    content,
)
sys.stdout.write(content)
