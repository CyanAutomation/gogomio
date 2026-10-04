#!/usr/bin/env python3
"""Reject manual Docker tags that can impersonate immutable release versions."""

import sys


def validate_dispatch_tags(value):
    tags = [tag.strip() for tag in value.split(",")]
    if not tags or any(not tag for tag in tags):
        raise ValueError("at least one non-empty Docker tag is required")
    if any(tag.lower().startswith("v") for tag in tags):
        raise ValueError("manual runs cannot assign v-prefixed release tags")
    if len(set(tags)) != len(tags):
        raise ValueError("duplicate Docker tags are not allowed")
    return ",".join(tags)


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 1:
        print("usage: validate_docker_dispatch_tags.py TAGS", file=sys.stderr)
        return 2
    try:
        print(validate_dispatch_tags(args[0]))
    except ValueError as error:
        print("Invalid manual Docker tags: {}".format(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
