import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def workflow(name):
    return (ROOT / ".github" / "workflows" / name).read_text()


def workflow_step(text, name):
    match = re.search(
        rf"(?ms)^      - name: {re.escape(name)}\n(?P<body>.*?)(?=^      - name: |\Z)",
        text,
    )
    if match is None:
        raise AssertionError(f"workflow step {name!r} was not found")
    return match.group("body")


class WorkflowContractTests(unittest.TestCase):
    def test_kaseki_wrappers_share_a_main_only_normal_pr_workflow(self):
        reusable = workflow("kaseki-sweep.yml")
        for name in ("kaseki-docs.yaml", "kaseki-dry.yaml"):
            with self.subTest(workflow=name):
                caller = workflow(name)
                self.assertIn("uses: ./.github/workflows/kaseki-sweep.yml", caller)
                self.assertIn("if: github.ref == 'refs/heads/main'", caller)
        self.assertIn("if: github.ref == 'refs/heads/main'", reusable)
        self.assertIn("REF: main", reusable)
        self.assertIn('publishMode: "pr"', reusable)
        self.assertIn("KASEKI_ALLOWED_HOSTS", reusable)
        job_environment = reusable.split("\njobs:\n  sweep:", 1)[1].split("\n    steps:", 1)[0]
        self.assertNotIn("KASEKI_API_TOKEN:", job_environment)
        self.assertNotIn("draft", reusable.lower())

    def test_kaseki_health_probe_uses_the_validated_controller_url(self):
        reusable = workflow("kaseki-sweep.yml")
        validation = workflow_step(reusable, "Validate Kaseki configuration")
        health = workflow_step(reusable, "Verify controller health")

        self.assertIn("id: validate_kaseki_configuration", validation)
        self.assertIn('echo "base_url=$validated_url" >> "$GITHUB_OUTPUT"', validation)
        self.assertIn(
            "base-url: ${{ steps.validate_kaseki_configuration.outputs.base_url }}",
            health,
        )

    def test_docker_publisher_validates_scans_verifies_and_attests_before_promotion(self):
        text = workflow("build-multiarch.yml")
        self.assertIn("github.event_name != 'workflow_dispatch' || github.ref == 'refs/heads/main'", text)
        self.assertIn("concurrency:", text)
        self.assertIn("sbom: true", text)
        self.assertIn("provenance: true", text)
        self.assertIn("needs: [validate, build, verify, scan]", text)
        self.assertIn("docker buildx imagetools create", text)
        self.assertIn("IMAGE_DIGEST", text)
        self.assertIn("validate_release_tag_event.py", text)
        self.assertRegex(text, r"(?m)^\s+timeout-minutes:")

    def test_release_workflow_checks_tag_commit_is_on_main(self):
        text = workflow("goreleaser.yml")
        self.assertIn("validate_release_source.py", text)
        self.assertIn("fetch-depth: 0", text)
        self.assertIn("git fetch origin main", text)
        self.assertIn("validate_release_tag_event.py", text)

    def test_pr_workflows_do_not_persist_checkout_credentials(self):
        for name in ("benchmark.yml", "code-coverage-test.yml"):
            with self.subTest(workflow=name):
                self.assertGreaterEqual(workflow(name).count("persist-credentials: false"), 1)
        benchmark = workflow("benchmark.yml")
        self.assertIn("github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'", benchmark)

    def test_coverage_artifact_is_attempted_after_failures(self):
        # Contract: TC-CI-01 (docs/testing/test-contracts.md).
        step = workflow_step(workflow("code-coverage-test.yml"), "Store coverage artifact")
        self.assertIn("if: always()", step)
        self.assertIn("uses: actions/upload-artifact", step)
        self.assertIn("path: coverage.out", step)

    def test_javascript_unit_tests_are_run_in_ci(self):
        # Contract: TC-WEB-02 (docs/testing/test-contracts.md).
        command = "node --test internal/web/aspect-ratio.test.js internal/web/diagnostics-dialog.test.js"
        for name in ("code-coverage-test.yml", "build-multiarch.yml"):
            with self.subTest(workflow=name):
                self.assertIn(command, workflow(name))


if __name__ == "__main__":
    unittest.main()
