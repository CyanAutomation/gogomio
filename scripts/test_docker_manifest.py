import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from verify_docker_manifest import manifest_platform_digests, validate_manifest


class DockerManifestTests(unittest.TestCase):
    def test_accepts_manifest_with_both_linux_architectures(self):
        manifest = {
            "manifests": [
                {"platform": {"os": "linux", "architecture": "amd64"}},
                {"platform": {"os": "linux", "architecture": "arm64"}},
            ]
        }

        self.assertEqual(validate_manifest(manifest), {"amd64", "arm64"})

    def test_returns_the_digest_for_each_required_linux_architecture(self):
        manifest = {
            "manifests": [
                {
                    "digest": "sha256:" + "a" * 64,
                    "platform": {"os": "linux", "architecture": "amd64"},
                },
                {
                    "digest": "sha256:" + "b" * 64,
                    "platform": {"os": "linux", "architecture": "arm64"},
                },
            ]
        }

        self.assertEqual(
            manifest_platform_digests(manifest),
            {"amd64": "sha256:" + "a" * 64, "arm64": "sha256:" + "b" * 64},
        )

    def test_rejects_platform_descriptors_without_a_valid_digest(self):
        manifest = {
            "manifests": [
                {
                    "digest": "not-a-digest",
                    "platform": {"os": "linux", "architecture": "amd64"},
                },
                {
                    "digest": "sha256:" + "b" * 64,
                    "platform": {"os": "linux", "architecture": "arm64"},
                },
            ]
        }

        with self.assertRaisesRegex(ValueError, "amd64.*digest"):
            manifest_platform_digests(manifest)

    def test_rejects_ambiguous_duplicate_platform_descriptors(self):
        manifest = {
            "manifests": [
                {
                    "digest": "sha256:" + "a" * 64,
                    "platform": {"os": "linux", "architecture": "amd64"},
                },
                {
                    "digest": "sha256:" + "b" * 64,
                    "platform": {"os": "linux", "architecture": "amd64"},
                },
                {
                    "digest": "sha256:" + "c" * 64,
                    "platform": {"os": "linux", "architecture": "arm64"},
                },
            ]
        }

        with self.assertRaisesRegex(ValueError, "duplicate.*amd64"):
            manifest_platform_digests(manifest)

    def test_cli_writes_platform_digests_as_github_outputs(self):
        manifest = {
            "manifests": [
                {
                    "digest": "sha256:" + "a" * 64,
                    "platform": {"os": "linux", "architecture": "amd64"},
                },
                {
                    "digest": "sha256:" + "b" * 64,
                    "platform": {"os": "linux", "architecture": "arm64"},
                },
            ]
        }
        script = Path(__file__).with_name("verify_docker_manifest.py")

        with tempfile.TemporaryDirectory() as directory:
            output_path = Path(directory) / "github-output"
            result = subprocess.run(
                [sys.executable, str(script), "--github-output", str(output_path)],
                input=json.dumps(manifest),
                text=True,
                capture_output=True,
                check=False,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                output_path.read_text().splitlines(),
                ["amd64_digest=sha256:" + "a" * 64, "arm64_digest=sha256:" + "b" * 64],
            )

    def test_rejects_missing_architecture(self):
        manifest = {
            "manifests": [
                {"platform": {"os": "linux", "architecture": "amd64"}},
                {"platform": {"os": "linux", "architecture": "unknown"}},
            ]
        }

        with self.assertRaisesRegex(ValueError, "arm64"):
            validate_manifest(manifest)

    def test_ignores_non_linux_manifests(self):
        manifest = {
            "manifests": [
                {"platform": {"os": "linux", "architecture": "amd64"}},
                {"platform": {"os": "windows", "architecture": "arm64"}},
            ]
        }

        with self.assertRaisesRegex(ValueError, "arm64"):
            validate_manifest(manifest)

    def test_rejects_malformed_manifest_objects(self):
        with self.assertRaisesRegex(ValueError, "manifest list"):
            validate_manifest({"manifests": "not a list"})


if __name__ == "__main__":
    unittest.main()
