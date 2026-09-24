import unittest

from check_benchstat import count_comparisons, find_regressions


class BenchstatRegressionTests(unittest.TestCase):
    def test_flags_only_significant_increases_above_threshold(self):
        report = """\
FastPath  1.00µ ± 1%  1.11µ ± 1%  +11.00% (p=0.001 n=10)
SmallChange  1.00µ ± 1%  1.09µ ± 1%  +9.00% (p=0.001 n=10)
Noise  1.00µ ± 1%  1.20µ ± 1%  +20.00% (p=0.200 n=10)
Improvement  1.20µ ± 1%  1.00µ ± 1%  -16.67% (p=0.001 n=10)
geomean  1.00µ  1.11µ  +11.00%
"""

        regressions = find_regressions(report, threshold_percent=10.0, alpha=0.05)

        self.assertEqual([item.line.split()[0] for item in regressions], ["FastPath"])

    def test_does_not_treat_missing_or_unrecognized_statistics_as_a_regression(self):
        report = """\
NoSamples  1.00µ ± 1%  1.20µ ± 1%  +20.00%
NotSignificant  1.00µ ± 1%  1.20µ ± 1%  +20.00% (p=1.000 n=10)
"""

        self.assertEqual(find_regressions(report), [])

    def test_counts_only_rows_with_benchstat_significance_results(self):
        report = """\
Comparable  1.00µ ± 1%  1.20µ ± 1%  +20.00% (p=0.001 n=10)
NotComparable  1.00µ ± 1%  1.20µ ± 1%  +20.00%
geomean  1.00µ  1.20µ  +20.00%
"""

        self.assertEqual(count_comparisons(report), 1)


if __name__ == "__main__":
    unittest.main()
