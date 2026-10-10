import subprocess
import sys
import unittest
from pathlib import Path

from enforce_coverage import CoverageParseError, CoverageThresholdError, validate_coverage


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "enforce_coverage.py"


class CoverageContractTests(unittest.TestCase):
    # Contract: TC-CI-03 (docs/testing/test-contracts.md).
    def test_accepts_coverage_at_or_above_the_threshold(self):
        for value in ("75.0%", "82.4%"):
            with self.subTest(value=value):
                self.assertEqual(validate_coverage(f"total: (statements) {value}\n"), float(value[:-1]))

    def test_rejects_coverage_below_the_threshold(self):
        with self.assertRaises(CoverageThresholdError):
            validate_coverage("total: (statements) 74.9%\n")

    def test_rejects_missing_or_malformed_total_coverage(self):
        for report in (
            "",
            "package/file.go:10.0%\n",
            "total: (statements) NaN%\n",
            "total: (statements) 75\n",
        ):
            with self.subTest(report=report), self.assertRaises(CoverageParseError):
                validate_coverage(report)

    def test_cli_fails_for_malformed_coverage_output(self):
        result = subprocess.run(
            [sys.executable, str(SCRIPT)],
            input="total: (statements) not-a-percent\n",
            text=True,
            capture_output=True,
            check=False,
        )

        self.assertEqual(result.returncode, 1)
        self.assertIn("Could not parse total coverage", result.stderr)

    def test_cli_fails_when_total_coverage_is_below_the_threshold(self):
        result = subprocess.run(
            [sys.executable, str(SCRIPT), "--minimum", "75"],
            input="total: (statements) 74.9%\n",
            text=True,
            capture_output=True,
            check=False,
        )

        self.assertEqual(result.returncode, 1)
        self.assertIn("below the 75.0% threshold", result.stderr)


if __name__ == "__main__":
    unittest.main()
