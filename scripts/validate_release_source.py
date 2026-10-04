#!/usr/bin/env python3
"""Ensure a release tag commit is reachable from the protected main branch."""

import subprocess
import sys


def is_ancestor(repository, commit, main_ref):
    result = subprocess.run(
        ["git", "merge-base", "--is-ancestor", commit, main_ref],
        cwd=repository,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    if result.returncode not in (0, 1):
        raise RuntimeError("could not compare release commit with main")
    return result.returncode == 0


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 2:
        print("usage: validate_release_source.py COMMIT MAIN_REF", file=sys.stderr)
        return 2
    try:
        valid = is_ancestor(".", args[0], args[1])
    except RuntimeError as error:
        print("Release source check failed: {}".format(error), file=sys.stderr)
        return 2
    if not valid:
        print("Release tag commit must be reachable from main.", file=sys.stderr)
        return 1
    print("Release commit is reachable from main.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
