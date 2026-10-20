import unittest

from validate_kaseki_changed_files import AllowlistValidationError, validate_changed_files


class KasekiChangedFilesTests(unittest.TestCase):
    def test_accepts_repo_root_and_recursive_glob_matches(self):
        analysis = {
            "changes": {
                "changedFiles": [
                    "README.md",
                    "docs/README.md",
                    "docs/guides/install.md",
                    "docs/guides/deep/example.md",
                ]
            }
        }

        self.assertEqual(
            validate_changed_files(analysis, "README.md,docs/**/*.md"),
            analysis["changes"]["changedFiles"],
        )

    def test_rejects_a_path_outside_the_allowlist(self):
        analysis = {"changes": {"changedFiles": ["README.md", "src/main.go"]}}

        with self.assertRaisesRegex(AllowlistValidationError, "src/main.go"):
            validate_changed_files(analysis, "README.md,docs/**/*.md")

    def test_fails_closed_when_analysis_has_no_changed_file_list(self):
        for analysis in ({}, {"changes": {}}, {"changes": {"changedFiles": "README.md"}}):
            with self.subTest(analysis=analysis):
                with self.assertRaises(AllowlistValidationError):
                    validate_changed_files(analysis, "README.md")

    def test_rejects_absolute_and_parent_traversal_paths(self):
        for path in ("/README.md", "../README.md", "docs/../../README.md"):
            with self.subTest(path=path):
                analysis = {"changes": {"changedFiles": [path]}}
                with self.assertRaises(AllowlistValidationError):
                    validate_changed_files(analysis, "**")


if __name__ == "__main__":
    unittest.main()
