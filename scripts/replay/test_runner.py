import json
from contextlib import redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
from pathlib import Path
import socket
import tempfile
import threading
import unittest

import runner


def make_workload():
    return {
        "schema_version": "1", "workload_id": "azure-llm-2023-code-000000001",
        "source_dataset": "azure-llm-2023", "source_record_id": "row-000000001",
        "arrival_offset_ms": 0, "queue": "llm", "tenant_id": "synthetic-tenant-0001",
        "task_type": "eval", "context_tokens": 12, "generated_tokens": 8,
        "duration_model": "none", "expected_outcome": "success", "seed": 7,
        "provenance": {"context_tokens": "OBSERVED", "tenant_id": "SYNTHETIC"},
    }


class FakeAPI(BaseHTTPRequestHandler):
    posts = 0
    idempotency = []
    status_code = 201
    get_state = "success"
    get_status_code = 200
    redirect_to = None
    disconnect_once = False
    auth = []
    body = None

    def log_message(self, *_):
        pass

    def do_POST(self):
        type(self).posts += 1
        type(self).auth.append(self.headers.get("Authorization"))
        type(self).idempotency.append(self.headers.get("Idempotency-Key"))
        length = int(self.headers.get("Content-Length", "0"))
        type(self).body = json.loads(self.rfile.read(length))
        if type(self).disconnect_once:
            type(self).disconnect_once = False
            self.connection.shutdown(socket.SHUT_RDWR)
            self.connection.close()
            return
        if type(self).redirect_to:
            self.send_response(307)
            self.send_header("Location", type(self).redirect_to)
            self.end_headers()
            return
        self.send_response(type(self).status_code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        body = {"id": "task-1", "status": "pending"} if type(self).status_code == 201 else {"error": {"code": "rejected"}}
        self.wfile.write(json.dumps(body).encode())

    def do_GET(self):
        self.send_response(type(self).get_status_code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        task = {"id": "task-1", "status": type(self).get_state, "created_at": "2026-10-12T00:00:00Z",
                "started_at": "2026-10-12T00:00:01Z", "finished_at": "2026-10-12T00:00:02Z",
                "attempt": 1, "lease_version": 2, "result": {"artifacts": {"case_results_uri": "artifact://run/cases.jsonl"}}}
        self.wfile.write(json.dumps(task).encode())


class ReplayRunnerTests(unittest.TestCase):
    def setUp(self):
        FakeAPI.posts = 0
        FakeAPI.idempotency = []
        FakeAPI.auth = []
        FakeAPI.status_code = 201
        FakeAPI.get_state = "success"
        FakeAPI.get_status_code = 200
        FakeAPI.redirect_to = None
        FakeAPI.disconnect_once = False
        FakeAPI.body = None
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), FakeAPI)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.input = self.root / "input.jsonl"
        self.manifest = self.root / "manifest.json"
        self.checkpoint = self.root / "checkpoint.sqlite"
        self.output = self.root / "results.jsonl"
        self.input.write_text(json.dumps(make_workload(), sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")
        self.manifest.write_text(json.dumps({"status": "GENERATED_NOT_EXECUTED", "generated_records": 1,
            "generated_output_sha256": runner.sha256_file(self.input), "source_sha256": "a" * 64}), encoding="utf-8")
        self.base = ["--input", str(self.input), "--manifest", str(self.manifest), "--api-url",
            f"http://127.0.0.1:{self.server.server_port}", "--allow-http-local", "--run-id", "test-run",
            "--dataset-uri", "artifact://replay/dataset.jsonl", "--dataset-sha256", "b" * 64,
            "--checkpoint", str(self.checkpoint), "--output", str(self.output), "--poll-interval", "0.01"]

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)
        self.temp.cleanup()

    def args(self, *extra):
        return runner.build_parser().parse_args(self.base + list(extra))

    def run_silently(self, args):
        with redirect_stdout(io.StringIO()):
            return runner.run(args)

    def test_real_api_create_poll_and_evidence(self):
        import os
        os.environ["TASKLAB_TEST_TOKEN"] = "short-lived-test-token"
        args = self.args("--token-env", "TASKLAB_TEST_TOKEN")
        self.assertEqual(self.run_silently(args), 0)
        self.assertEqual(FakeAPI.posts, 1)
        self.assertEqual(FakeAPI.auth, ["Bearer short-lived-test-token"])
        self.assertEqual(len(FakeAPI.idempotency[0]), 64)
        self.assertEqual(FakeAPI.body["payload"]["data"]["execution_mode"], "gateway")
        self.assertNotIn("synthetic-tenant-0001", FakeAPI.body)
        summary = json.loads((self.root / "results.jsonl.summary.json").read_text())
        self.assertEqual(summary["mode"], "CORE_E2E_BENCHMARK")
        self.assertEqual(summary["terminal_success"], 1)
        result = json.loads(self.output.read_text())
        self.assertEqual(result["status"], "success")
        self.assertEqual(result["queue_wait_ms"], 1000)
        self.assertEqual(result["e2e_latency_ms"], 2000)
        self.assertEqual(result["lease_version"], 2)
        self.assertIn("artifact://run/cases.jsonl", json.dumps(result["task_result"]))
        del os.environ["TASKLAB_TEST_TOKEN"]

    def test_resume_does_not_resubmit_completed_logical_task(self):
        import os
        os.environ["TASKLAB_TEST_TOKEN"] = "test-token"
        args = self.args("--token-env", "TASKLAB_TEST_TOKEN")
        self.run_silently(args)
        self.assertEqual(self.run_silently(self.args("--token-env", "TASKLAB_TEST_TOKEN", "--resume")), 0)
        self.assertEqual(FakeAPI.posts, 1)
        self.assertEqual(json.loads((self.root / "results.jsonl.summary.json").read_text())["terminal_success"], 1)
        del os.environ["TASKLAB_TEST_TOKEN"]

    def test_manifest_hash_mismatch_is_rejected_before_api(self):
        self.manifest.write_text(json.dumps({"status": "GENERATED_NOT_EXECUTED", "generated_records": 1,
            "generated_output_sha256": "0" * 64, "source_sha256": "a" * 64}), encoding="utf-8")
        with self.assertRaisesRegex(runner.RunnerError, "SHA256"):
            self.run_silently(self.args())
        self.assertEqual(FakeAPI.posts, 0)

    def test_unsorted_arrivals_are_rejected(self):
        first = make_workload()
        second = make_workload()
        second["workload_id"] = "second"
        second["arrival_offset_ms"] = -1
        self.input.write_text("\n".join(json.dumps(row) for row in (first, second)) + "\n", encoding="utf-8")
        self.manifest.write_text(json.dumps({"status": "GENERATED_NOT_EXECUTED", "generated_records": 2,
            "generated_output_sha256": runner.sha256_file(self.input), "source_sha256": "a" * 64}), encoding="utf-8")
        with self.assertRaisesRegex(runner.RunnerError, "nondecreasing"):
            runner.load_and_validate(self.input, self.manifest, None, 100)

    def test_non_2xx_is_recorded_as_rejected_not_success(self):
        import os
        FakeAPI.status_code = 422
        os.environ["TASKLAB_TEST_TOKEN"] = "test-token"
        args = self.args("--token-env", "TASKLAB_TEST_TOKEN")
        self.assertEqual(self.run_silently(args), 3)
        summary = json.loads((self.root / "results.jsonl.summary.json").read_text())
        self.assertEqual(summary["rejected_requests"], 1)
        self.assertEqual(summary["terminal_success"], 0)
        del os.environ["TASKLAB_TEST_TOKEN"]

    def test_checkpoint_rejects_different_run(self):
        db = runner.initialize_checkpoint(self.checkpoint, "a" * 64, "run-a", "c" * 64)
        db.close()
        with self.assertRaisesRegex(runner.RunnerError, "different input hash or run id"):
            runner.initialize_checkpoint(self.checkpoint, "a" * 64, "run-b", "c" * 64)

    def test_invalid_non_loopback_http_is_rejected(self):
        args = self.args()
        args.api_url = "http://example.invalid"
        with self.assertRaisesRegex(runner.RunnerError, "loopback"):
            self.run_silently(args)

    def test_invalid_json_and_schema_are_rejected(self):
        self.input.write_text("{not-json}\n", encoding="utf-8")
        self.manifest.write_text(json.dumps({"status": "GENERATED_NOT_EXECUTED", "generated_records": 1,
            "generated_output_sha256": runner.sha256_file(self.input), "source_sha256": "a" * 64}), encoding="utf-8")
        with self.assertRaisesRegex(runner.RunnerError, "invalid JSON"):
            runner.load_and_validate(self.input, self.manifest, None, 100)
        self.input.write_text(json.dumps({"schema_version": "99"}) + "\n", encoding="utf-8")
        self.manifest.write_text(json.dumps({"status": "GENERATED_NOT_EXECUTED", "generated_records": 1,
            "generated_output_sha256": runner.sha256_file(self.input), "source_sha256": "a" * 64}), encoding="utf-8")
        with self.assertRaisesRegex(runner.RunnerError, "schema fields"):
            runner.load_and_validate(self.input, self.manifest, None, 100)

    def test_task_that_never_finishes_is_bounded(self):
        import os
        FakeAPI.get_state = "running"
        os.environ["TASKLAB_TEST_TOKEN"] = "test-token"
        args = self.args("--token-env", "TASKLAB_TEST_TOKEN", "--task-deadline", "0.08", "--poll-interval", "0.01")
        self.assertEqual(self.run_silently(args), 3)
        summary = json.loads((self.root / "results.jsonl.summary.json").read_text())
        self.assertEqual(summary["unfinished_tasks"], 1)
        del os.environ["TASKLAB_TEST_TOKEN"]

    def test_cancel_before_submission_does_not_call_api(self):
        from threading import Event
        from types import SimpleNamespace
        cancel = Event()
        cancel.set()
        args = self.args()
        with self.assertRaisesRegex(runner.RunnerError, "cancelled"):
            runner.create_and_wait(make_workload(), 1, args, "c" * 64, "not-logged", cancel)
        self.assertEqual(FakeAPI.posts, 0)

    def test_redirect_is_not_followed(self):
        import os
        FakeAPI.redirect_to = "http://127.0.0.1:1/collect"
        os.environ["TASKLAB_TEST_TOKEN"] = "test-token"
        args = self.args("--token-env", "TASKLAB_TEST_TOKEN")
        self.assertEqual(self.run_silently(args), 3)
        del os.environ["TASKLAB_TEST_TOKEN"]

    def test_existing_result_is_never_overwritten_without_resume(self):
        self.output.write_text("prior evidence\n", encoding="utf-8")
        with self.assertRaisesRegex(runner.RunnerError, "refusing to overwrite"):
            self.run_silently(self.args())
        self.assertEqual(self.output.read_text(encoding="utf-8"), "prior evidence\n")

    def test_lost_create_response_retries_same_idempotency_key(self):
        import os
        FakeAPI.disconnect_once = True
        os.environ["TASKLAB_TEST_TOKEN"] = "test-token"
        args = self.args("--token-env", "TASKLAB_TEST_TOKEN", "--poll-interval", "0.01")
        self.assertEqual(self.run_silently(args), 0)
        self.assertEqual(FakeAPI.posts, 2)
        self.assertEqual(FakeAPI.idempotency[0], FakeAPI.idempotency[1])
        result = json.loads(self.output.read_text(encoding="utf-8"))
        self.assertEqual(result["client_retries"], 1)
        del os.environ["TASKLAB_TEST_TOKEN"]

    def test_max_runtime_rejects_overlong_schedule_before_submission(self):
        args = self.args("--max-runtime", "0.001")
        item = make_workload()
        item["arrival_offset_ms"] = 10
        self.input.write_text(json.dumps(item) + "\n", encoding="utf-8")
        self.manifest.write_text(json.dumps({"status": "GENERATED_NOT_EXECUTED", "generated_records": 1,
            "generated_output_sha256": runner.sha256_file(self.input), "source_sha256": "a" * 64}), encoding="utf-8")
        with self.assertRaisesRegex(runner.RunnerError, "arrival span"):
            self.run_silently(args)
        self.assertEqual(FakeAPI.posts, 0)


if __name__ == "__main__":
    unittest.main()
