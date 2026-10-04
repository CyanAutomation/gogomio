import unittest

from validate_kaseki_url import KasekiURLValidationError, validate_base_url


class ValidateKasekiURLTests(unittest.TestCase):
    def test_accepts_https_url_on_exact_allowlisted_host(self):
        self.assertEqual(
            validate_base_url("https://Kaseki.example.test/api/", "kaseki.example.test"),
            "https://kaseki.example.test/api",
        )

    def test_accepts_explicit_allowlisted_port(self):
        self.assertEqual(
            validate_base_url("https://kaseki.example.test:8443", "kaseki.example.test:8443"),
            "https://kaseki.example.test:8443",
        )

    def test_rejects_non_https_and_missing_host(self):
        for value in ("http://kaseki.example.test", "https:///runs"):
            with self.subTest(value=value), self.assertRaises(KasekiURLValidationError):
                validate_base_url(value, "kaseki.example.test")

    def test_rejects_credentials_query_fragment_and_whitespace(self):
        for value in (
            "https://user@kaseki.example.test",
            "https://kaseki.example.test?next=evil",
            "https://kaseki.example.test#fragment",
            "https://kaseki.example.test/path with space",
        ):
            with self.subTest(value=value), self.assertRaises(KasekiURLValidationError):
                validate_base_url(value, "kaseki.example.test")

    def test_rejects_encoded_parent_path_segments(self):
        for path in (
            "/../api",
            "/%2e%2e/api",
            "/%2E%2E/api",
            "/%2e./api",
            "/%2f../api",
            "/%5c../api",
            "/..\\api",
            "/%252e%252e/api",
            "/%25252e%25252e/api",
        ):
            with self.subTest(path=path), self.assertRaises(KasekiURLValidationError):
                validate_base_url("https://kaseki.example.test{}".format(path), "kaseki.example.test")

    def test_accepts_encoded_non_traversal_path(self):
        self.assertEqual(
            validate_base_url("https://kaseki.example.test/api%20v1", "kaseki.example.test"),
            "https://kaseki.example.test/api%20v1",
        )

    def test_rejects_excessively_nested_path_encoding(self):
        encoded_dot = "%2e"
        for _ in range(20):
            encoded_dot = encoded_dot.replace("%", "%25")
        path = "/{}{}/api".format(encoded_dot, encoded_dot)
        with self.assertRaises(KasekiURLValidationError):
            validate_base_url("https://kaseki.example.test{}".format(path), "kaseki.example.test")

    def test_rejects_unlisted_hosts_and_malformed_allowlist(self):
        for value, allowed in (
            ("https://attacker.example.test", "kaseki.example.test"),
            ("https://kaseki.example.test", "*.example.test"),
            ("https://kaseki.example.test", ""),
        ):
            with self.subTest(value=value, allowed=allowed), self.assertRaises(KasekiURLValidationError):
                validate_base_url(value, allowed)


if __name__ == "__main__":
    unittest.main()
