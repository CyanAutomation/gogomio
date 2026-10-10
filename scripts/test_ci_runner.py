import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RUNNER = ROOT / "scripts" / "test-ci-helpers.sh"


class CIRunnerTests(unittest.TestCase):
    # Keeps the JavaScript contracts TC-WEB-02 and TC-WEB-03 in the test run.
    def test_runner_executes_each_registered_helper_suite(self):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            calls_file = temp / "calls.log"
            for tool in ("python3", "bash", "node"):
                shim = temp / tool
                shim.write_text(
                    "#!/bin/sh\n"
                    f'printf "%s\\t%s\\n" "{tool}" "$*" >> "$TEST_CALLS"\n',
                    encoding="utf-8",
                )
                shim.chmod(0o755)

            environment = os.environ.copy()
            environment["PATH"] = f"{temp}:{environment['PATH']}"
            environment["TEST_CALLS"] = str(calls_file)
            result = subprocess.run(
                ["/bin/bash", str(RUNNER)],
                cwd=ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                calls_file.read_text(encoding="utf-8").splitlines(),
                [
                    "python3\tscripts/check_skills.py",
                    "bash\tscripts/test-docker-tags.sh",
                    "bash\tscripts/test-docker-go-tests.sh",
                    "python3\t-B -m unittest discover -s scripts -p test_*.py",
                    "node\t--test internal/web/aspect-ratio.test.js internal/web/diagnostics-dialog.test.js",
                ],
            )

    def test_go_ci_runner_runs_vet_and_race_tests(self):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            calls_file = temp / "go-calls.log"
            go_shim = temp / "go"
            go_shim.write_text(
                "#!/bin/sh\n"
                'printf "%s\\n" "$*" >> "$TEST_CALLS"\n',
                encoding="utf-8",
            )
            go_shim.chmod(0o755)

            environment = os.environ.copy()
            environment["PATH"] = f"{temp}:{environment['PATH']}"
            environment["TEST_CALLS"] = str(calls_file)
            result = subprocess.run(
                ["/bin/bash", str(ROOT / "scripts" / "test-go-ci.sh")],
                cwd=ROOT,
                env=environment,
                text=True,
                capture_output=True,
                check=False,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                calls_file.read_text(encoding="utf-8").splitlines(),
                ["vet ./...", "test ./... -race"],
            )


if __name__ == "__main__":
    unittest.main()
