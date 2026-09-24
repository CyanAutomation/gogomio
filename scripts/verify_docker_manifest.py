#!/usr/bin/env python3
"""Check that a Docker manifest contains the required Linux architectures."""

import json
import sys


REQUIRED_ARCHITECTURES = {"amd64", "arm64"}


def validate_manifest(manifest):
    if not isinstance(manifest, dict) or not isinstance(manifest.get("manifests"), list):
        raise ValueError("expected a manifest list")

    manifests = manifest["manifests"]
    if not manifests:
        raise ValueError("manifest list is empty")

    linux_architectures = set()
    for descriptor in manifests:
        if not isinstance(descriptor, dict):
            continue
        platform = descriptor.get("platform")
        if not isinstance(platform, dict) or platform.get("os") != "linux":
            continue
        architecture = platform.get("architecture")
        if isinstance(architecture, str):
            linux_architectures.add(architecture)

    missing = REQUIRED_ARCHITECTURES - linux_architectures
    if missing:
        raise ValueError(
            "missing required linux architecture(s): {}".format(", ".join(sorted(missing)))
        )
    return linux_architectures


def main():
    try:
        manifest = json.load(sys.stdin)
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        print("Invalid manifest JSON: {}".format(error), file=sys.stderr)
        return 1

    try:
        architectures = validate_manifest(manifest)
    except ValueError as error:
        print("Invalid multi-architecture manifest: {}".format(error), file=sys.stderr)
        return 1

    print(
        "Verified linux/amd64 and linux/arm64 (found: {}).".format(
            ", ".join(sorted(architectures))
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
