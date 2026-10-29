#!/usr/bin/env python3
"""Check source-backed contracts between GoGoMio docs and implementation."""

import json
import re
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REMOVAL_DATE = re.compile(
    r"\b(?:will be removed|scheduled for removal|planned for removal)\s+(?:in|for)\s+(v\d+(?:\.\d+){1,2})\b",
    re.IGNORECASE,
)
GO_ROUTE = re.compile(r'HandleFunc\("([A-Z]+)\s+([^\"]+)"')
YAML_PATHS = re.compile(r"^paths:\s*(?:#.*)?$")
YAML_PATH = re.compile(r"^  (/[^:]+):\s*$")
YAML_METHOD = re.compile(r"^    (get|post|put|delete|patch|options|head):\s*$", re.IGNORECASE)
HTTP_METHODS = {"get", "post", "put", "delete", "patch", "options", "head"}
LICENSE_IDENTIFIERS = {
    "MIT License": "MIT",
    "Apache License, Version 2.0": "Apache-2.0",
    "BSD 3-Clause License": "BSD-3-Clause",
}


def _yaml_operations(path: Path) -> set[tuple[str, str]]:
    operations: set[tuple[str, str]] = set()
    current_path = None
    in_paths = False
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if not line[0].isspace():
            in_paths = YAML_PATHS.match(line) is not None
            current_path = None
            continue
        if not in_paths:
            continue
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
        if method.lower() in HTTP_METHODS
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

        json_spec = json.loads((ROOT / "docs/reference/openapi.json").read_text(encoding="utf-8"))
        yaml_spec = (ROOT / "docs/reference/openapi.yaml").read_text(encoding="utf-8")
        yaml_license = re.search(r"(?m)^  license:\n    name: ([^\n]+)$", yaml_spec)

        self.assertEqual(json_spec["info"]["license"]["name"], expected)
        self.assertIsNotNone(yaml_license, "OpenAPI YAML must declare its license")
        self.assertEqual(yaml_license.group(1), expected)

    def test_openapi_removal_dates_match_readme_policy(self):
        documents = {
            "README.md": (ROOT / "README.md").read_text(encoding="utf-8"),
            "OpenAPI YAML": (ROOT / "docs/reference/openapi.yaml").read_text(encoding="utf-8"),
            "OpenAPI JSON": (ROOT / "docs/reference/openapi.json").read_text(encoding="utf-8"),
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
        yaml_operations = _yaml_operations(ROOT / "docs/reference/openapi.yaml")
        json_operations = _json_operations(ROOT / "docs/reference/openapi.json")

        self.assertEqual(yaml_operations, json_operations)

    def test_camera_config_schema_documents_mock_camera_mode(self):
        json_spec = json.loads((ROOT / "docs/reference/openapi.json").read_text(encoding="utf-8"))
        yaml_spec = (ROOT / "docs/reference/openapi.yaml").read_text(encoding="utf-8")

        properties = json_spec["components"]["schemas"]["api.CameraConfigResponse"]["properties"]
        self.assertIn("mock_camera", properties)
        self.assertIn("mock_camera:", yaml_spec)

    def test_openapi_documents_use_openapi_3_without_swagger_2_markers(self):
        json_spec = json.loads((ROOT / "docs/reference/openapi.json").read_text(encoding="utf-8"))
        yaml_spec = (ROOT / "docs/reference/openapi.yaml").read_text(encoding="utf-8")

        self.assertEqual(json_spec["openapi"], "3.0.3")
        self.assertNotIn("swagger", json_spec)
        self.assertFalse({"host", "basePath", "schemes", "definitions", "securityDefinitions"} & json_spec.keys())
        self.assertRegex(yaml_spec, r"(?m)^openapi: 3\.0\.3$")
        self.assertNotRegex(yaml_spec, r"(?m)^swagger:")
        self.assertIn("#/components/schemas/", json.dumps(json_spec))
        refs = re.findall(r'"\$ref": "([^"]+)"', json.dumps(json_spec))
        for ref in refs:
            self.assertTrue(ref.startswith("#/components/schemas/"), ref)
            self.assertIn(ref.removeprefix("#/components/schemas/"), json_spec["components"]["schemas"])
        for path_item in json_spec["paths"].values():
            for operation in path_item.values():
                if not isinstance(operation, dict):
                    continue
                self.assertNotIn("consumes", operation)
                self.assertNotIn("produces", operation)
        self.assertEqual(
            json_spec["paths"]["/v1/api/settings"]["post"]["requestBody"]["content"]
            ["application/json"]["schema"]["$ref"],
            "#/components/schemas/api.SettingsUpdateRequest",
        )
        snapshot_schema = json_spec["paths"]["/v1/snapshot.jpg"]["get"]["responses"]["200"]
        self.assertEqual(snapshot_schema["content"]["image/jpeg"]["schema"], {
            "type": "string",
            "format": "binary",
        })
        self.assertEqual(json_spec["components"]["securitySchemes"]["basicAuth"], {
            "type": "http",
            "scheme": "basic",
        })
        expected_settings_description = (
            'Settings update request. Example: {"settings":{"brightness":100,"contrast":120}}'
        )
        for method in ("put", "post"):
            self.assertEqual(
                json_spec["paths"]["/v1/api/settings"][method]["requestBody"]["description"],
                expected_settings_description,
            )
        self.assertEqual(yaml_spec.count(f"description: '{expected_settings_description}'"), 2)

        snapshot_responses = json_spec["paths"]["/v1/snapshot.jpg"]["get"]["responses"]
        self.assertEqual(snapshot_responses["429"]["content"], {
            "application/json": {
                "schema": {"$ref": "#/components/schemas/api.ErrorResponse"},
            },
        })
        self.assertEqual(snapshot_responses["503"]["content"], {
            "text/plain": {"schema": {"type": "string"}},
        })
        snapshot_yaml = yaml_spec.split("  /v1/snapshot.jpg:\n", 1)[1].split("\ncomponents:\n", 1)[0]
        after_429 = snapshot_yaml.split("        '429':\n", 1)[1]
        yaml_429, yaml_503 = after_429.split("        '503':\n", 1)
        self.assertIn("            application/json:\n", yaml_429)
        self.assertNotIn("image/jpeg:", yaml_429)
        self.assertIn("            text/plain:\n", yaml_503)
        self.assertIn("                type: string", yaml_503)

    def test_yaml_operations_ignore_method_like_keys_outside_paths(self):
        yaml_spec = '''openapi: 3.0.3
paths:
  /widgets:
    get:
      responses: {}
x-example:
  schema:
    post:
      summary: arbitrary extension data
'''
        with tempfile.TemporaryDirectory() as temp_dir:
            spec_path = Path(temp_dir) / "openapi.yaml"
            spec_path.write_text(yaml_spec, encoding="utf-8")

            self.assertEqual(_yaml_operations(spec_path), {("get", "/widgets")})

    def test_json_operations_ignore_path_level_parameters(self):
        json_spec = {
            "openapi": "3.0.3",
            "paths": {
                "/widgets": {
                    "parameters": [{"name": "id", "in": "path"}],
                    "get": {"responses": {}},
                }
            },
        }
        with tempfile.TemporaryDirectory() as temp_dir:
            spec_path = Path(temp_dir) / "openapi.json"
            spec_path.write_text(json.dumps(json_spec), encoding="utf-8")

            self.assertEqual(_json_operations(spec_path), {("get", "/widgets")})

    def test_documented_api_operations_are_registered_in_go_handlers(self):
        handlers = (ROOT / "internal/api/handlers.go").read_text(encoding="utf-8")
        registered = {
            (method.lower(), route)
            for method, route in GO_ROUTE.findall(handlers)
        }
        documented = _json_operations(ROOT / "docs/reference/openapi.json")
        missing = sorted(documented - registered)

        self.assertEqual(missing, [], f"OpenAPI operations missing from Go handlers: {missing}")


if __name__ == "__main__":
    unittest.main()
