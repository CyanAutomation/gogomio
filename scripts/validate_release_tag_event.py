#!/usr/bin/env python3
"""Allow release workflows only for newly created Git tags."""

import re
import sys


TAG_BEFORE_SHA_RE = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$")


def is_new_tag_event(previous_sha):
    if previous_sha == "" or previous_sha in ("0" * 40, "0" * 64):
        return True
    if not TAG_BEFORE_SHA_RE.fullmatch(previous_sha):
        raise ValueError("GitHub supplied an invalid previous tag SHA")
    raise ValueError("release tags are immutable; this tag already existed")


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 1:
        print("usage: validate_release_tag_event.py PREVIOUS_SHA", file=sys.stderr)
        return 2
    try:
        is_new_tag_event(args[0])
    except ValueError as error:
        print("Release tag event rejected: {}".format(error), file=sys.stderr)
        return 1
    print("Release tag was newly created.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
