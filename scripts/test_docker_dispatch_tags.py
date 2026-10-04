import unittest

from validate_docker_dispatch_tags import validate_dispatch_tags


class DockerDispatchTagTests(unittest.TestCase):
    def test_allows_aliases_and_custom_tags(self):
        self.assertEqual(validate_dispatch_tags(" latest,manual-test "), "latest,manual-test")

    def test_rejects_release_style_tags_and_duplicates(self):
        for value in ("v1.2.3", "v1-custom", "latest,latest"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate_dispatch_tags(value)

    def test_rejects_empty_tags(self):
        for value in ("", "latest,", ",latest"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate_dispatch_tags(value)


if __name__ == "__main__":
    unittest.main()
