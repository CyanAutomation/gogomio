#!/usr/bin/env python3
"""Validate v-prefixed release versions that can also be Docker tags."""

import re
import sys


NUMERIC_IDENTIFIER = r"(?:0|[1-9][0-9]*)"
NON_NUMERIC_IDENTIFIER = r"(?:[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)"
PRERELEASE_IDENTIFIER = r"(?:{}|{})".format(
    NUMERIC_IDENTIFIER, NON_NUMERIC_IDENTIFIER
)
RELEASE_TAG_RE = re.compile(
    r"^v{}\.{}\.{}(?:-{}(?:\.{})*)?$".format(
        NUMERIC_IDENTIFIER,
        NUMERIC_IDENTIFIER,
        NUMERIC_IDENTIFIER,
        PRERELEASE_IDENTIFIER,
        PRERELEASE_IDENTIFIER,
    )
)
MAX_DOCKER_TAG_LENGTH = 128
ARM64_SUFFIX = "-arm64"
MAX_RELEASE_TAG_LENGTH = MAX_DOCKER_TAG_LENGTH - len(ARM64_SUFFIX)


def is_valid_release_tag(tag):
    return (
        len(tag) <= MAX_RELEASE_TAG_LENGTH
        and RELEASE_TAG_RE.fullmatch(tag) is not None
    )


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 1:
        print("usage: validate_release_tag.py TAG", file=sys.stderr)
        return 2
    if not is_valid_release_tag(args[0]):
        print(
            "Release tag must be a v-prefixed version with valid numeric core "
            "and optional prerelease, fit within 122 characters to allow the "
            "-arm64 Docker tag, and omit build metadata.",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
