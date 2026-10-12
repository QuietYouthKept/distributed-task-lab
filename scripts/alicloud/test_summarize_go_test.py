import json
import pathlib
import subprocess
import sys
import tempfile
import unittest


SCRIPT = pathlib.Path(__file__).resolve().with_name("summarize-go-test.py")
PACKAGE = "github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
REQUIRED = "TestRuntimeRoleLeastPrivilege"


def event(action, test=None):
    result = {"Action": action, "Package": PACKAGE}
    if test is not None:
        result["Test"] = test
    return json.dumps(result)


class GoTestSummaryTests(unittest.TestCase):
    def summarize(self, events):
        with tempfile.TemporaryDirectory() as directory:
            source = pathlib.Path(directory) / "events.jsonl"
            destination = pathlib.Path(directory) / "summary.json"
            source.write_text("\n".join(events) + "\n", encoding="utf-8")
            result = subprocess.run(
                [sys.executable, str(SCRIPT), str(source), str(destination), "--require-test", REQUIRED],
                check=False,
                capture_output=True,
                text=True,
            )
            summary = json.loads(destination.read_text(encoding="utf-8"))
            return result, summary

    def test_complete_required_suite_passes(self):
        result, summary = self.summarize(
            [event("run", REQUIRED), event("pass", REQUIRED), event("pass")]
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(summary["pass_claimed"])
        self.assertEqual(summary["missing_required_tests"], [])

    def test_any_skip_fails_even_when_go_package_passes(self):
        result, summary = self.summarize(
            [event("run", REQUIRED), event("pass", REQUIRED), event("skip", "TestSomeDBCase"), event("pass")]
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(summary["pass_claimed"])
        self.assertEqual(summary["tests"]["skip"], 1)

    def test_missing_required_database_test_fails(self):
        result, summary = self.summarize([event("run", "TestOther"), event("pass", "TestOther"), event("pass")])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(summary["missing_required_tests"], [REQUIRED])

    def test_zero_executed_tests_fails(self):
        result, summary = self.summarize([event("pass")])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(summary["tests"]["run"], 0)

    def test_failure_event_fails(self):
        result, summary = self.summarize([event("run", REQUIRED), event("fail", REQUIRED), event("fail")])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(summary["tests"]["fail"], 1)


if __name__ == "__main__":
    unittest.main()
