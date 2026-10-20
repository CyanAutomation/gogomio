#!/usr/bin/env python3
"""Verify Kaseki's reported changed files against the requested path allowlist."""

import json
import re
import sys
from pathlib import PurePosixPath


class AllowlistValidationError(ValueError):
    """Raised when a Kaseki result cannot be proven to respect its allowlist."""


def glob_to_regex(pattern):
    """Translate Kaseki's repo-relative glob syntax into an anchored regex."""
    if (
        not pattern
        or pattern.startswith("/")
        or "\\" in pattern
        or "\x00" in pattern
        or any(part in ("", ".", "..") for part in pattern.split("/"))
    ):
        raise AllowlistValidationError("allowlist contains an invalid repo-relative pattern")

    regex = []
    index = 0
    while index < len(pattern):
        char = pattern[index]
        next_char = pattern[index + 1] if index + 1 < len(pattern) else ""
        next_next_char = pattern[index + 2] if index + 2 < len(pattern) else ""
        if char == "*" and next_char == "*":
            if next_next_char == "/":
                regex.append("(?:[^/]+/)*")
                index += 3
            else:
                regex.append(".*")
                index += 2
        elif char == "*":
            regex.append("[^/]*")
            index += 1
        elif char == "?":
            regex.append("[^/]")
            index += 1
        else:
            regex.append(re.escape(char))
            index += 1

    return re.compile("^" + "".join(regex) + "$")


def parse_allowlist(value):
    patterns = [item.strip() for item in value.split(",") if item.strip()]
    if not patterns:
        raise AllowlistValidationError("allowlist must contain at least one path pattern")
    return [glob_to_regex(pattern) for pattern in patterns]


def validate_changed_files(analysis, allowlist):
    """Return changed paths if every path matches a requested allowlist pattern."""
    if not isinstance(analysis, dict):
        raise AllowlistValidationError("Kaseki analysis response must be a JSON object")

    changes = analysis.get("changes")
    files = changes.get("changedFiles") if isinstance(changes, dict) else None
    if not isinstance(files, list) or any(not isinstance(path, str) for path in files):
        raise AllowlistValidationError(
            "Kaseki analysis did not include a valid changes.changedFiles list"
        )

    patterns = parse_allowlist(allowlist)
    for path in files:
        if (
            not path
            or path.startswith("/")
            or "\\" in path
            or "\x00" in path
            or any(part in ("", ".", "..") for part in path.split("/"))
            or PurePosixPath(path).is_absolute()
        ):
            raise AllowlistValidationError("Kaseki reported an invalid repo-relative path")
        if not any(pattern.fullmatch(path) for pattern in patterns):
            raise AllowlistValidationError(
                "Kaseki changed a file outside the allowlist: {}".format(path)
            )
    return files


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 2:
        print(
            "usage: validate_kaseki_changed_files.py ANALYSIS_JSON ALLOWLIST",
            file=sys.stderr,
        )
        return 2

    try:
        with open(args[0], encoding="utf-8") as analysis_file:
            analysis = json.load(analysis_file)
        files = validate_changed_files(analysis, args[1])
    except (OSError, json.JSONDecodeError, AllowlistValidationError) as error:
        print("Kaseki changed-file check failed: {}".format(error), file=sys.stderr)
        return 1

    print("Kaseki changed-file check passed ({} file(s)).".format(len(files)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
