import unittest

from validate_release_tag_event import is_new_tag_event


class ReleaseTagEventTests(unittest.TestCase):
    def test_accepts_new_tag_events(self):
        self.assertTrue(is_new_tag_event(""))
        self.assertTrue(is_new_tag_event("0" * 40))
        self.assertTrue(is_new_tag_event("0" * 64))

    def test_rejects_tag_updates_and_malformed_before_sha(self):
        for value in ("a" * 40, "f" * 64, "0" * 39, "invalid"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                is_new_tag_event(value)


if __name__ == "__main__":
    unittest.main()
