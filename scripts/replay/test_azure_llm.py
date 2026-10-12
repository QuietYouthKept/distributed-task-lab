import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))
import azure_llm


class AzureLLMReplayTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.source = self.root / "trace_code.csv"
        self.source.write_text(
            "TIMESTAMP,ContextTokens,GeneratedTokens\n"
            "2023-11-16 18:17:03.9799600,12,3\n"
            "2023-11-16 18:17:04.0319600,20,4\n",
            encoding="utf-8",
        )

    def tearDown(self):
        self.temp.cleanup()

    def args(self, output, **overrides):
        values = {
            "input": self.source,
            "output": output,
            "count": 2,
            "seed": 42,
            "time_scale": azure_llm.Decimal("1"),
            "queues": "llm",
            "tenants": 1,
            "max_records": 1000,
            "max_output_bytes": 1_000_000,
        }
        values.update(overrides)
        return type("Args", (), values)()

    def test_replay_is_byte_deterministic_for_same_input_and_seed(self):
        first = self.root / "one.jsonl"
        second = self.root / "two.jsonl"
        azure_llm.generate(self.args(first))
        azure_llm.generate(self.args(second))
        self.assertEqual(first.read_bytes(), second.read_bytes())
        self.assertEqual(
            hashlib.sha256(first.read_bytes()).hexdigest(),
            hashlib.sha256(second.read_bytes()).hexdigest(),
        )

    def test_observed_tokens_and_provenance_are_preserved(self):
        output = self.root / "replay.jsonl"
        manifest = azure_llm.generate(self.args(output))
        records = [json.loads(line) for line in output.read_text().splitlines()]
        self.assertEqual([(r["context_tokens"], r["generated_tokens"]) for r in records], [(12, 3), (20, 4)])
        self.assertEqual(records[0]["provenance"]["context_tokens"], "OBSERVED")
        self.assertEqual(records[0]["provenance"]["tenant_id"], "SYNTHETIC")
        self.assertEqual(manifest["status"], "GENERATED_NOT_EXECUTED")
        self.assertEqual(manifest["generated_output_sha256"], hashlib.sha256(output.read_bytes()).hexdigest())

    def test_repeated_source_pass_gets_later_arrival_offsets(self):
        output = self.root / "repeated.jsonl"
        azure_llm.generate(self.args(output, count=5))
        records = [json.loads(line) for line in output.read_text().splitlines()]
        arrivals = [r["arrival_offset_ms"] for r in records]
        self.assertEqual(arrivals, sorted(arrivals))
        self.assertGreater(arrivals[2], arrivals[1])
        self.assertEqual(records[2]["source_record_id"], records[0]["source_record_id"])

    def test_out_of_order_timestamp_is_rejected_without_publishing(self):
        self.source.write_text(
            "TIMESTAMP,ContextTokens,GeneratedTokens\n"
            "2023-11-16 18:17:04.0000000,1,1\n"
            "2023-11-16 18:17:03.0000000,1,1\n",
            encoding="utf-8",
        )
        output = self.root / "bad.jsonl"
        with self.assertRaisesRegex(azure_llm.ReplayError, "not nondecreasing"):
            azure_llm.generate(self.args(output))
        self.assertFalse(output.exists())
        self.assertFalse(list(self.root.glob(".*.tmp")))

    def test_invalid_token_and_oversized_output_are_rejected(self):
        self.source.write_text(
            "TIMESTAMP,ContextTokens,GeneratedTokens\n"
            "2023-11-16 18:17:03.0000000,-1,1\n",
            encoding="utf-8",
        )
        with self.assertRaisesRegex(azure_llm.ReplayError, "non-negative"):
            azure_llm.generate(self.args(self.root / "invalid.jsonl"))
        self.source.write_text(
            "TIMESTAMP,ContextTokens,GeneratedTokens\n"
            "2023-11-16 18:17:03.0000000,100,100\n",
            encoding="utf-8",
        )
        output = self.root / "oversize.jsonl"
        with self.assertRaisesRegex(azure_llm.ReplayError, "exceeds"):
            azure_llm.generate(self.args(output, count=10, max_output_bytes=100))
        self.assertFalse(output.exists())
        self.assertFalse(list(self.root.glob(".*.tmp")))

    def test_existing_output_is_never_overwritten(self):
        output = self.root / "existing.jsonl"
        output.write_text("preserve me", encoding="utf-8")
        with self.assertRaisesRegex(azure_llm.ReplayError, "refusing to overwrite"):
            azure_llm.generate(self.args(output))
        self.assertEqual(output.read_text(encoding="utf-8"), "preserve me")

    def test_concurrent_output_creation_is_not_overwritten(self):
        output = self.root / "raced.jsonl"
        link = azure_llm.os.link

        def create_racing_output(source, destination):
            Path(destination).write_text("concurrent writer", encoding="utf-8")
            return link(source, destination)

        with mock.patch.object(azure_llm.os, "link", side_effect=create_racing_output):
            with self.assertRaises(FileExistsError):
                azure_llm.generate(self.args(output))
        self.assertEqual(output.read_text(encoding="utf-8"), "concurrent writer")
        self.assertFalse(list(self.root.glob(".*.tmp")))


if __name__ == "__main__":
    unittest.main()
