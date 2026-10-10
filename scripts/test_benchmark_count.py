import os
import subprocess
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "benchmark-count.sh"


class BenchmarkCountTests(unittest.TestCase):
    def count_for(self, event_name):
        environment = os.environ.copy()
        environment["GITHUB_EVENT_NAME"] = event_name
        result = subprocess.run(
            ["bash", str(SCRIPT)],
            text=True,
            capture_output=True,
            check=True,
            env=environment,
        )
        return result.stdout.strip()

    def test_pull_requests_use_fewer_samples(self):
        # Contract: TC-CI-02 (docs/testing/test-contracts.md).
        self.assertEqual(self.count_for("pull_request"), "5")

    def test_non_pull_request_runs_use_full_sample_count(self):
        for event_name in ("push", "schedule", "workflow_dispatch"):
            with self.subTest(event_name=event_name):
                self.assertEqual(self.count_for(event_name), "10")


if __name__ == "__main__":
    unittest.main()
