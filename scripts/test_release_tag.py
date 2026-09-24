import unittest

from validate_release_tag import is_valid_release_tag


class ReleaseTagValidationTests(unittest.TestCase):
    def test_accepts_stable_and_prerelease_versions(self):
        for tag in ("v0.1.0", "v1.2.3-rc.1", "v2.0.0-preview-2.4"):
            with self.subTest(tag=tag):
                self.assertTrue(is_valid_release_tag(tag))

    def test_rejects_malformed_versions(self):
        for tag in (
            "1.2.3",
            "v01.2.3",
            "v1.02.3",
            "v1.2.03",
            "v1.2.3-",
            "v1.2.3-.1",
            "v1.2.3-foo..bar",
            "v1.2.3-01",
            "v1.2.3+build.1",
            "v1.2.3; touch /tmp/unexpected",
        ):
            with self.subTest(tag=tag):
                self.assertFalse(is_valid_release_tag(tag))

    def test_enforces_docker_tag_length_with_arm64_suffix_room(self):
        max_tag = "v1.2.3-" + "a" * 115
        too_long_tag = "v1.2.3-" + "a" * 116

        self.assertEqual(len(max_tag), 122)
        self.assertTrue(is_valid_release_tag(max_tag))
        self.assertFalse(is_valid_release_tag(too_long_tag))


if __name__ == "__main__":
    unittest.main()
