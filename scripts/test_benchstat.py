import io
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from tempfile import TemporaryDirectory

from check_benchstat import count_comparisons, find_regressions, main


class BenchstatRegressionTests(unittest.TestCase):
    def test_parses_two_file_sample_counts_and_gates_by_metric_direction(self):
        report = """\
name old time/op new time/op delta
SlowPath 100ns/op ± 1% 120ns/op ± 2% +20.00% (p=0.001 n=10+10)
NearThreshold 100ns/op ± 1% 114ns/op ± 2% +14.00% (p=0.001 n=10+10)
Noise 100ns/op ± 1% 130ns/op ± 2% +30.00% (p=0.200 n=10+10)
Improvement 120ns/op ± 2% 100ns/op ± 1% -16.67% (p=0.001 n=10+10)

name old MB/s new MB/s delta
ThroughputUp 100MB/s ± 1% 130MB/s ± 2% +30.00% (p=0.001 n=10+10)
ThroughputDown 100MB/s ± 1% 80MB/s ± 2% -20.00% (p=0.001 n=10+10)
"""

        self.assertEqual(count_comparisons(report), 6)
        regressions = find_regressions(report, threshold_percent=15.0, alpha=0.05)

        self.assertEqual(
            [item.line.split()[0] for item in regressions],
            ["SlowPath", "ThroughputDown"],
        )
        self.assertEqual([item.metric for item in regressions], ["time/op", "mb/s"])

    def test_parses_benchstat_box_table_headers_and_single_sample_counts(self):
        report = """\
goos: linux
goarch: amd64
│ baseline/bench-current.txt │ bench-current.txt │
│           sec/op           │ sec/op vs base    │
WriteMultipartFrame-4 24.73n ± 1% 24.79n ± 2% ~ (p=0.643 n=10)
WriteMultipartFrameLegacy-4 168.8n ± 2% 174.3n ± 0% +20.00% (p=0.000 n=10)
geomean 127.1n 127.7n +0.47%

│ baseline/bench-current.txt │ bench-current.txt │
│            B/op            │ B/op vs base      │
WriteMultipartFrame-4 0.000 ± 0% 0.000 ± 0% ~ (p=1.000 n=10)
"""

        self.assertEqual(count_comparisons(report), 3)
        regressions = find_regressions(report, threshold_percent=15.0, alpha=0.05)
        self.assertEqual([item.line.split()[0] for item in regressions], ["WriteMultipartFrameLegacy-4"])
        self.assertEqual([item.metric for item in regressions], ["sec/op"])

    def test_uses_holm_correction_across_comparable_rows(self):
        report = """\
name old time/op new time/op delta
First 100ns/op ± 1% 130ns/op ± 2% +30.00% (p=0.020 n=10+10)
Second 100ns/op ± 1% 130ns/op ± 2% +30.00% (p=0.020 n=10+10)
Third 100ns/op ± 1% 130ns/op ± 2% +30.00% (p=0.020 n=10+10)
"""

        self.assertEqual(
            find_regressions(report, threshold_percent=15.0, alpha=0.05), []
        )

    def test_ignores_metrics_without_a_known_direction(self):
        report = """\
name old custom/op new custom/op delta
Custom 1unit/op ± 1% 2unit/op ± 2% +50.00% (p=0.001 n=10+10)
"""

        self.assertEqual(count_comparisons(report), 1)
        self.assertEqual(find_regressions(report), [])

    def test_reports_unclassified_metrics_without_gating_them(self):
        report = """\
name old time/op new time/op delta
Stable 100ns/op ± 1% 100ns/op ± 1% ~ (p=0.900 n=10+10)

name old custom/op new custom/op delta
Custom 1unit/op ± 1% 2unit/op ± 2% +50.00% (p=0.001 n=10+10)
"""

        with TemporaryDirectory() as directory:
            report_path = Path(directory) / "benchstat.txt"
            report_path.write_text(report, encoding="utf-8")
            output = io.StringIO()
            with redirect_stdout(output):
                status = main([str(report_path)])

        self.assertEqual(status, 0)
        self.assertIn("custom/op", output.getvalue())

    def test_ignores_rows_without_benchstat_significance_data(self):
        report = """\
name old time/op new time/op delta
NoSamples 100ns/op ± 1% 130ns/op ± 2% +30.00%
NoDelta 100ns/op ± 1% 130ns/op ± 2% ~ (p=0.200 n=10+10)
"""

        self.assertEqual(count_comparisons(report), 1)
        self.assertEqual(find_regressions(report), [])


if __name__ == "__main__":
    unittest.main()
