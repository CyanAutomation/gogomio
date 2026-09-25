#!/usr/bin/env python3
"""Fail for statistically significant benchmark regressions above a threshold."""

import argparse
import math
import re
import sys
from collections import namedtuple
from pathlib import Path


Comparison = namedtuple("Comparison", ["line", "metric", "delta_percent", "p_value"])
Regression = namedtuple(
    "Regression", ["line", "metric", "delta_percent", "p_value", "adjusted_p_value"]
)

HEADER_RE = re.compile(
    r"^\s*name\s+old\s+(?P<metric>\S+)\s+new\s+\S+\s+delta\s*$",
    re.IGNORECASE,
)
STATISTIC_RE = re.compile(
    r"\(p=(?P<p_value><\s*[0-9]+(?:\.[0-9]+)?|"
    r"[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)\s+"
    r"n=(?P<old_samples>[0-9]+)\+(?P<new_samples>[0-9]+)\)"
)
DELTA_RE = re.compile(
    r"(?P<delta>[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+|Inf))%$"
)

LOWER_IS_BETTER = {"time/op", "b/op", "bytes/op", "allocs/op", "allocations/op"}
HIGHER_IS_BETTER = {
    "b/s",
    "kb/s",
    "mb/s",
    "gb/s",
    "kib/s",
    "mib/s",
    "gib/s",
    "frames/op",
    "frames/s",
}
SUPPORTED_METRICS = LOWER_IS_BETTER | HIGHER_IS_BETTER


def parse_p_value(raw_value):
    """Parse benchstat's displayed p-value conservatively."""
    if raw_value.startswith("<"):
        return float(raw_value[1:].strip())

    value = float(raw_value)
    if value == 0:
        # Text output rounds p-values to three decimal places. Treat 0.000 as
        # its largest value consistent with that rounding, not as exact zero.
        return 0.0005
    return value


def parse_comparisons(report):
    """Return benchstat comparison rows, carrying the metric from each table."""
    comparisons = []
    metric = None

    for line in report.splitlines():
        header = HEADER_RE.match(line)
        if header is not None:
            metric = header.group("metric").lower()
            continue

        statistic = STATISTIC_RE.search(line)
        if statistic is None or metric is None:
            continue

        delta = DELTA_RE.search(line[: statistic.start()].rstrip())
        delta_percent = float(delta.group("delta")) if delta is not None else None
        comparisons.append(
            Comparison(
                line.strip(),
                metric,
                delta_percent,
                parse_p_value(statistic.group("p_value")),
            )
        )

    return comparisons


def count_comparisons(report):
    return len(parse_comparisons(report))


def holm_adjusted_p_values(comparisons):
    """Return Holm-Bonferroni adjusted p-values in input order."""
    ordered = sorted(enumerate(comparisons), key=lambda item: item[1].p_value)
    adjusted = [1.0] * len(comparisons)
    running_max = 0.0
    count = len(comparisons)

    for rank, (index, comparison) in enumerate(ordered):
        candidate = min(1.0, (count - rank) * comparison.p_value)
        running_max = max(running_max, candidate)
        adjusted[index] = running_max

    return adjusted


def find_regressions(report, threshold_percent=15.0, alpha=0.05):
    comparisons = parse_comparisons(report)
    supported = [
        comparison
        for comparison in comparisons
        if comparison.metric in SUPPORTED_METRICS
    ]
    adjusted_p_values = holm_adjusted_p_values(supported)
    regressions = []

    for comparison, adjusted_p_value in zip(supported, adjusted_p_values):
        if comparison.delta_percent is None:
            continue

        if comparison.metric in LOWER_IS_BETTER:
            is_regression = comparison.delta_percent > threshold_percent
        else:
            is_regression = comparison.delta_percent < -threshold_percent

        if is_regression and adjusted_p_value < alpha:
            regressions.append(
                Regression(
                    comparison.line,
                    comparison.metric,
                    comparison.delta_percent,
                    comparison.p_value,
                    adjusted_p_value,
                )
            )

    return regressions


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("report", type=Path, help="benchstat comparison output")
    parser.add_argument("--threshold-percent", type=float, default=15.0)
    parser.add_argument("--alpha", type=float, default=0.05)
    args = parser.parse_args(argv)
    if not math.isfinite(args.threshold_percent) or args.threshold_percent < 0:
        parser.error("--threshold-percent must be a finite non-negative number")
    if not math.isfinite(args.alpha) or not 0 <= args.alpha <= 1:
        parser.error("--alpha must be between 0 and 1")

    try:
        report = args.report.read_text(encoding="utf-8")
    except OSError as error:
        print("Could not read benchstat report: {}".format(error), file=sys.stderr)
        return 2

    comparisons = parse_comparisons(report)
    if not comparisons:
        print("No comparable benchmark statistics were produced.", file=sys.stderr)
        return 1

    supported_count = sum(
        1
        for comparison in comparisons
        if comparison.metric in SUPPORTED_METRICS
    )
    if not supported_count:
        print(
            "No benchmark metrics with a configured direction were found.",
            file=sys.stderr,
        )
        return 1

    unclassified_metrics = sorted(
        {
            comparison.metric
            for comparison in comparisons
            if comparison.metric not in SUPPORTED_METRICS
        }
    )
    if unclassified_metrics:
        print(
            "Unclassified metrics (informational only): {}".format(
                ", ".join(unclassified_metrics)
            )
        )

    regressions = find_regressions(report, args.threshold_percent, args.alpha)
    print(
        "Checked {} comparable rows across {} recognized metric rows; "
        "using Holm correction (alpha={:.3f}).".format(
            len(comparisons), supported_count, args.alpha
        )
    )

    if regressions:
        print(
            "Significant regressions above {:.2f}%:".format(
                args.threshold_percent
            ),
            file=sys.stderr,
        )
        for regression in regressions:
            print(
                "  {} (metric={}, delta={:+.2f}%, p={:.4g}, Holm p={:.4g})".format(
                    regression.line,
                    regression.metric,
                    regression.delta_percent,
                    regression.p_value,
                    regression.adjusted_p_value,
                ),
                file=sys.stderr,
            )
        return 1

    print(
        "No significant benchmark regression above {:.2f}% after Holm correction.".format(
            args.threshold_percent
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
