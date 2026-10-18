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


def workflow_job(text, name):
    match = re.search(
        rf"(?ms)^  {re.escape(name)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)",
        text,
    )
    if match is None:
        raise AssertionError(f"workflow job {name!r} was not found")
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
        for name, group in (
            ("kaseki-docs.yaml", "kaseki-docs-sweep"),
            ("kaseki-dry.yaml", "kaseki-dry-sweep"),
        ):
            with self.subTest(workflow=name):
                self.assertIn(
                    f"group: {group}-${{{{ github.repository }}}}-${{{{ github.ref == 'refs/heads/main' && 'main' || github.run_id }}}}",
                    workflow(name),
                )

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

    def test_docker_images_use_patched_go_and_upgrade_debian_base_packages(self):
        dockerfile = (ROOT / "Dockerfile").read_text()
        demo_dockerfile = (ROOT / "Dockerfile.demo").read_text()
        cloudbuild = (ROOT / "cloudbuild.yaml").read_text()
        go_mod = (ROOT / "go.mod").read_text()

        go_version = "1.27.2"
        self.assertIn(f"FROM golang:{go_version}-alpine3.23 AS builder", dockerfile)
        self.assertIn(f"FROM golang:{go_version}-alpine3.23 AS build", demo_dockerfile)
        self.assertNotIn("GO_IMAGE=golang:", cloudbuild)
        self.assertNotIn("BUILDER_BASE_IMAGE=golang:", cloudbuild)
        self.assertIn(f"go {go_version}", go_mod)
        self.assertIn("apt-get upgrade -y --quiet --no-install-recommends", dockerfile)

    def test_docker_images_are_refreshed_and_scanned_per_architecture(self):
        text = workflow("build-multiarch.yml")
        build = workflow_step(text, "Build candidate and generate attestations")
        resolve = workflow_step(text, "Resolve candidate platform digests")
        scan = workflow_job(text, "scan")

        self.assertIn("schedule:", text)
        self.assertIn("pull: true", build)
        self.assertIn("no-cache: ${{ github.event_name == 'schedule' }}", build)
        self.assertIn("verify_docker_manifest.py --github-output", resolve)
        self.assertIn("steps.platform-digests.outputs.amd64_digest", workflow_job(text, "build"))
        self.assertIn("steps.platform-digests.outputs.arm64_digest", workflow_job(text, "build"))
        self.assertIn("architecture: [amd64, arm64]", scan)
        self.assertIn("matrix.architecture == 'amd64'", scan)
        self.assertIn("version: v0.75.0", scan)

    def test_release_workflow_checks_tag_commit_is_on_main(self):
        text = workflow("goreleaser.yml")
        self.assertIn("validate_release_source.py", text)
        self.assertIn("fetch-depth: 0", text)
        self.assertIn("git fetch origin main", text)
        self.assertIn("validate_release_tag_event.py", text)

    def test_pr_workflows_do_not_persist_checkout_credentials(self):
        self.assertGreaterEqual(
            workflow("code-coverage-test.yml").count("persist-credentials: false"),
            1,
        )

    def test_pr_ci_cancels_obsolete_runs_without_cancelling_other_events(self):
        text = workflow("code-coverage-test.yml")
        self.assertIn(
            "group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}-${{ github.event_name }}",
            text,
        )
        self.assertIn(
            "cancel-in-progress: ${{ github.event_name == 'pull_request' }}",
            text,
        )

    def test_docker_release_tags_are_not_coalesced_or_used_for_latest(self):
        text = workflow("build-multiarch.yml")
        self.assertIn(
            "group: docker-publish-${{ github.repository }}-${{ github.ref_type == 'tag' && github.ref_name || (github.event_name == 'workflow_dispatch' && github.ref != 'refs/heads/main' && github.run_id) || 'latest' }}",
            text,
        )
        self.assertIn('requested_tags="${GITHUB_REF_NAME}"', text)
        self.assertNotIn('requested_tags="${GITHUB_REF_NAME},latest"', text)

    def test_pull_only_docker_jobs_use_read_only_credentials_and_scan_has_no_github_permissions(self):
        text = workflow("build-multiarch.yml")
        for name in ("verify", "scan"):
            with self.subTest(job=name):
                job = workflow_job(text, name)
                self.assertIn("DOCKERHUB_READONLY_USERNAME", job)
                self.assertIn("DOCKERHUB_READONLY_TOKEN", job)
                self.assertNotIn("secrets.DOCKER_USERNAME", job)
                self.assertNotIn("secrets.DOCKER_PASSWORD", job)
        scan = workflow_job(text, "scan")
        self.assertIn("permissions: {}", scan)

    def test_kaseki_poll_deadline_precedes_controller_and_job_timeouts(self):
        reusable = workflow("kaseki-sweep.yml")
        wait = workflow_step(reusable, "Wait for Kaseki completion")
        self.assertIn("timeoutSeconds: 10800", reusable)
        self.assertIn("POLL_TIMEOUT_SECONDS=10500", wait)
        self.assertIn("poll_started_at=$SECONDS", wait)
        self.assertIn("No terminal status within 175 minutes", wait)
        self.assertNotIn("seq 1 185", wait)

    def test_ci_workflows_invoke_shared_test_runners(self):
        for name in ("code-coverage-test.yml", "build-multiarch.yml"):
            with self.subTest(workflow=name):
                self.assertIn("bash scripts/test-ci-helpers.sh", workflow(name))

        for name in ("build-multiarch.yml", "goreleaser.yml"):
            with self.subTest(workflow=name):
                self.assertIn("bash scripts/test-go-ci.sh", workflow(name))

        for name in ("code-coverage-test.yml", "build-multiarch.yml", "goreleaser.yml"):
            with self.subTest(node_workflow=name):
                self.assertIn("actions/setup-node@", workflow(name))
                self.assertIn("node-version: 22", workflow(name))

        docker_build = workflow_job(workflow("build-multiarch.yml"), "build")
        self.assertIn("npm run build:web", docker_build)
        self.assertLess(docker_build.index("npm run build:web"), docker_build.index("docker/build-push-action"))
        go_releaser = (ROOT / ".goreleaser.yml").read_text()
        self.assertIn("- cmd: npm run build:web", go_releaser)

        cloudbuild = (ROOT / "cloudbuild.yaml").read_text()
        self.assertIn("id: build-web-ui", cloudbuild)
        self.assertIn("npm run build:web", cloudbuild)
        self.assertIn("waitFor:\n      - build-web-ui", cloudbuild)

        lint = workflow_step(
            workflow("code-coverage-test.yml"), "Lint GitHub Actions workflows"
        )
        self.assertIn("actionlint", lint)

    def test_coverage_workflow_uses_executable_threshold_checker(self):
        step = workflow_step(workflow("code-coverage-test.yml"), "Enforce coverage threshold (≥75%)")
        self.assertIn("set -euo pipefail", step)
        self.assertIn(
            "go tool cover -func=coverage.out | python3 scripts/enforce_coverage.py --minimum 75",
            step,
        )

    def test_coverage_artifact_is_attempted_after_failures(self):
        # Contract: TC-CI-01 (docs/testing/test-contracts.md).
        step = workflow_step(workflow("code-coverage-test.yml"), "Store coverage artifact")
        self.assertIn("if: always()", step)
        self.assertIn("uses: actions/upload-artifact", step)
        self.assertIn("path: coverage.out", step)

if __name__ == "__main__":
    unittest.main()
