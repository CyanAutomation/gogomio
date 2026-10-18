#!/usr/bin/env python3
"""Check source-backed contracts between GoGoMio docs and implementation."""

import json
import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REMOVAL_DATE = re.compile(
    r"\b(?:will be removed|scheduled for removal|planned for removal)\s+(?:in|for)\s+(v\d+(?:\.\d+){1,2})\b",
    re.IGNORECASE,
)
GO_ROUTE = re.compile(r'HandleFunc\("([A-Z]+)\s+([^\"]+)"')
YAML_PATH = re.compile(r"^  (/[^:]+):\s*$")
YAML_METHOD = re.compile(r"^    (get|post|put|delete|patch|options|head):\s*$", re.IGNORECASE)
LICENSE_IDENTIFIERS = {
    "MIT License": "MIT",
    "Apache License, Version 2.0": "Apache-2.0",
    "BSD 3-Clause License": "BSD-3-Clause",
}


def _yaml_operations(path: Path) -> set[tuple[str, str]]:
    operations: set[tuple[str, str]] = set()
    current_path = None
    for line in path.read_text(encoding="utf-8").splitlines():
        path_match = YAML_PATH.match(line)
        if path_match:
            current_path = path_match.group(1)
            continue
        if current_path:
            method_match = YAML_METHOD.match(line)
            if method_match:
                operations.add((method_match.group(1).lower(), current_path))
    return operations


def _json_operations(path: Path) -> set[tuple[str, str]]:
    spec = json.loads(path.read_text(encoding="utf-8"))
    return {
        (method.lower(), route)
        for route, operations in spec["paths"].items()
        for method in operations
    }


class DocumentationContractTests(unittest.TestCase):
    def test_readme_license_matches_canonical_license_heading(self):
        license_heading = (ROOT / "LICENSE").read_text(encoding="utf-8").splitlines()[0]
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        match = re.search(r"This project is licensed under the \[([^]]+)\]\(\./LICENSE\)", readme)

        self.assertIsNotNone(match, "README must name and link the project license")
        self.assertEqual(match.group(1), license_heading)

    def test_openapi_license_metadata_matches_canonical_license(self):
        license_heading = (ROOT / "LICENSE").read_text(encoding="utf-8").splitlines()[0]
        self.assertIn(license_heading, LICENSE_IDENTIFIERS, "add the canonical SPDX identifier mapping")
        expected = LICENSE_IDENTIFIERS[license_heading]

        json_spec = json.loads((ROOT / "docs/reference/swagger.json").read_text(encoding="utf-8"))
        yaml_spec = (ROOT / "docs/reference/swagger.yaml").read_text(encoding="utf-8")
        yaml_license = re.search(r"(?m)^  license:\n    name: ([^\n]+)$", yaml_spec)

        self.assertEqual(json_spec["info"]["license"]["name"], expected)
        self.assertIsNotNone(yaml_license, "OpenAPI YAML must declare its license")
        self.assertEqual(yaml_license.group(1), expected)

    def test_openapi_removal_dates_match_readme_policy(self):
        documents = {
            "README.md": (ROOT / "README.md").read_text(encoding="utf-8"),
            "OpenAPI YAML": (ROOT / "docs/reference/swagger.yaml").read_text(encoding="utf-8"),
            "OpenAPI JSON": (ROOT / "docs/reference/swagger.json").read_text(encoding="utf-8"),
        }
        dates = {name: set(REMOVAL_DATE.findall(contents)) for name, contents in documents.items()}

        self.assertEqual(
            dates["OpenAPI YAML"], dates["README.md"],
            "OpenAPI YAML removal dates must be stated by the README policy",
        )
        self.assertEqual(
            dates["OpenAPI JSON"], dates["README.md"],
            "OpenAPI JSON removal dates must be stated by the README policy",
        )

    def test_openapi_paths_and_methods_match_between_formats(self):
        yaml_operations = _yaml_operations(ROOT / "docs/reference/swagger.yaml")
        json_operations = _json_operations(ROOT / "docs/reference/swagger.json")

        self.assertEqual(yaml_operations, json_operations)

    def test_documented_api_operations_are_registered_in_go_handlers(self):
        handlers = (ROOT / "internal/api/handlers.go").read_text(encoding="utf-8")
        registered = {
            (method.lower(), route)
            for method, route in GO_ROUTE.findall(handlers)
        }
        documented = _json_operations(ROOT / "docs/reference/swagger.json")
        missing = sorted(documented - registered)

        self.assertEqual(missing, [], f"OpenAPI operations missing from Go handlers: {missing}")


if __name__ == "__main__":
    unittest.main()
