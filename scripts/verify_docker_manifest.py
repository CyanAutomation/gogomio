#!/usr/bin/env python3
"""Check that a Docker manifest contains the required Linux architectures."""

import argparse
import json
import re
import sys


REQUIRED_ARCHITECTURES = {"amd64", "arm64"}
IMAGE_DIGEST_PATTERN = re.compile(r"sha256:[0-9a-f]{64}\Z")


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


def manifest_platform_digests(manifest):
    """Return the immutable image digest for each required Linux architecture."""
    validate_manifest(manifest)

    platform_digests = {}
    for descriptor in manifest["manifests"]:
        if not isinstance(descriptor, dict):
            continue
        platform = descriptor.get("platform")
        if not isinstance(platform, dict) or platform.get("os") != "linux":
            continue

        architecture = platform.get("architecture")
        if architecture not in REQUIRED_ARCHITECTURES:
            continue

        digest = descriptor.get("digest")
        if not isinstance(digest, str) or not IMAGE_DIGEST_PATTERN.fullmatch(digest):
            raise ValueError(
                "manifest for linux/{} has an invalid image digest".format(architecture)
            )
        if architecture in platform_digests:
            raise ValueError(
                "manifest has duplicate descriptors for linux/{}".format(architecture)
            )
        platform_digests[architecture] = digest

    return platform_digests


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--github-output",
        help="append per-architecture image digests to a GitHub Actions output file",
    )
    args = parser.parse_args()

    try:
        manifest = json.load(sys.stdin)
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        print("Invalid manifest JSON: {}".format(error), file=sys.stderr)
        return 1

    try:
        platform_digests = manifest_platform_digests(manifest)
    except ValueError as error:
        print("Invalid multi-architecture manifest: {}".format(error), file=sys.stderr)
        return 1

    print(
        "Verified linux/amd64 and linux/arm64 platform digests."
    )
    if args.github_output:
        try:
            with open(args.github_output, "a", encoding="utf-8") as output:
                for architecture in sorted(platform_digests):
                    print(
                        "{}_digest={}".format(architecture, platform_digests[architecture]),
                        file=output,
                    )
        except OSError as error:
            print("Could not write platform digests: {}".format(error), file=sys.stderr)
            return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
