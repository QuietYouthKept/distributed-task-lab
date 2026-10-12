import os
import hashlib
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import soak


class SoakRunnerTests(unittest.TestCase):
    def test_linux_memory_parser_uses_memavailable(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "meminfo"
            path.write_text("MemTotal: 8000000 kB\nMemAvailable: 6291456 kB\n", encoding="ascii")
            self.assertEqual(soak.linux_memory_mib(path), 6144)

    def test_linux_rss_parser_reads_child_rss(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "42" / "status"
            path.parent.mkdir()
            path.write_text("Name:\tpython\nVmRSS:\t2048 kB\n", encoding="ascii")
            self.assertEqual(soak.linux_rss_mib(42, Path(directory)), 2)

    def test_run_id_must_be_explicitly_confirmed(self):
        args = SimpleNamespace(confirm_run_id="other", run_id="run-a")
        with self.assertRaises(soak.SoakError):
            soak.validate(args)

    def test_child_command_assigns_unique_iteration_paths(self):
        args = SimpleNamespace(run_id="safe-run", input=Path("input.jsonl"), manifest=Path("manifest.json"),
                               api_url="https://localhost", dataset_uri="artifact://data", dataset_sha256="a" * 64,
                               time_scale="1", submit_rate=2, concurrency=3, max_tasks=50,
                               max_output_bytes=1000, ca_file=None, source_dataset=None, allow_http_local=False,
                               token_env="TASKLAB_BEARER_TOKEN")
        first = soak.child_command(args, 1, Path("out"))
        second = soak.child_command(args, 2, Path("out"))
        self.assertIn("safe-run-00001", first)
        self.assertIn("safe-run-00002", second)
        self.assertNotEqual(first[first.index("--output") + 1], second[second.index("--output") + 1])

    def test_output_size_is_recursive_and_ignores_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "a").write_bytes(b"1234")
            (root / "nested").mkdir()
            (root / "nested" / "b").write_bytes(b"56")
            self.assertEqual(soak.directory_bytes(root), 6)

    def test_evidence_seal_hashes_manifest_and_resource_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = root / "manifest.json"
            manifest.write_text("{}\n", encoding="utf-8")
            soak.seal_evidence(root)
            entries = (root / "SHA256SUMS").read_text(encoding="ascii")
            self.assertIn(hashlib.sha256(manifest.read_bytes()).hexdigest() + "  manifest.json", entries)


if __name__ == "__main__":
    unittest.main()
