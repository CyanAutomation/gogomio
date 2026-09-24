import unittest

from verify_docker_manifest import validate_manifest


class DockerManifestTests(unittest.TestCase):
    def test_accepts_manifest_with_both_linux_architectures(self):
        manifest = {
            "manifests": [
                {"platform": {"os": "linux", "architecture": "amd64"}},
                {"platform": {"os": "linux", "architecture": "arm64"}},
            ]
        }

        self.assertEqual(validate_manifest(manifest), {"amd64", "arm64"})

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
