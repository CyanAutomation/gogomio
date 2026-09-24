#!/usr/bin/env python3
"""Fail when benchstat reports a significant metric increase above a threshold."""

import argparse
import re
import sys
from collections import namedtuple
from pathlib import Path


Regression = namedtuple("Regression", ["line", "delta_percent", "p_value"])
COMPARISON_RE = re.compile(
    r"(?P<delta>\+[0-9]+(?:\.[0-9]+)?)%\s+"
    r"\(p=(?P<p_value>[0-9]+(?:\.[0-9]+)?)\s+n=[0-9]+\)"
)
STATISTIC_RE = re.compile(r"\(p=[0-9]+(?:\.[0-9]+)?\s+n=[0-9]+\)")


def count_comparisons(report):
    return sum(1 for line in report.splitlines() if STATISTIC_RE.search(line))


def find_regressions(report, threshold_percent=10.0, alpha=0.05):
    regressions = []
    for line in report.splitlines():
        match = COMPARISON_RE.search(line)
        if match is None:
            continue

        delta_percent = float(match.group("delta")[1:])
        p_value = float(match.group("p_value"))
        if delta_percent > threshold_percent and p_value < alpha:
            regressions.append(Regression(line.strip(), delta_percent, p_value))
    return regressions


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("report", type=Path, help="benchstat comparison output")
    parser.add_argument("--threshold-percent", type=float, default=10.0)
    parser.add_argument("--alpha", type=float, default=0.05)
    args = parser.parse_args(argv)
    if args.threshold_percent < 0:
        parser.error("--threshold-percent must be non-negative")
    if not 0 <= args.alpha <= 1:
        parser.error("--alpha must be between 0 and 1")

    try:
        report = args.report.read_text(encoding="utf-8")
    except OSError as error:
        print("Could not read benchstat report: {}".format(error), file=sys.stderr)
        return 2

    if count_comparisons(report) == 0:
        print("No comparable benchmark statistics were produced.", file=sys.stderr)
        return 1

    regressions = find_regressions(report, args.threshold_percent, args.alpha)
    if regressions:
        print(
            "Significant benchmark increases above {:.2f}%:".format(
                args.threshold_percent
            ),
            file=sys.stderr,
        )
        for regression in regressions:
            print("  {}".format(regression.line), file=sys.stderr)
        return 1

    print(
        "No significant benchmark increase above {:.2f}%.".format(
            args.threshold_percent
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
