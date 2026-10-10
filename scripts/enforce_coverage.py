#!/usr/bin/env python3
"""Validate the total coverage report emitted by `go tool cover`."""

import argparse
import math
import re
import sys


class CoverageParseError(ValueError):
    """The coverage report does not contain a valid total percentage."""


class CoverageThresholdError(ValueError):
    """The report's total coverage is below the configured minimum."""


def parse_total_coverage(report):
    for line in report.splitlines():
        fields = line.split()
        if not fields or fields[0] != "total:":
            continue
        if len(fields) < 3 or not re.fullmatch(r"[0-9]+(?:\.[0-9]+)?%", fields[2]):
            raise CoverageParseError("total coverage is not a percentage")
        percentage = float(fields[2][:-1])
        if not math.isfinite(percentage) or not 0 <= percentage <= 100:
            raise CoverageParseError("total coverage must be between 0% and 100%")
        return percentage
    raise CoverageParseError("total coverage row was not found")


def validate_coverage(report, minimum=75.0):
    percentage = parse_total_coverage(report)
    if percentage < minimum:
        raise CoverageThresholdError(
            "Coverage {:.1f}% is below the {:.1f}% threshold".format(percentage, minimum)
        )
    return percentage


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--minimum", type=float, default=75.0)
    args = parser.parse_args(argv)

    report = sys.stdin.read()
    try:
        percentage = validate_coverage(report, args.minimum)
    except CoverageParseError:
        print("::error::Could not parse total coverage from go tool output.", file=sys.stderr)
        return 1
    except CoverageThresholdError as error:
        print("::error::{}".format(error), file=sys.stderr)
        return 1

    print("Total coverage: {:.1f}%".format(percentage))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
