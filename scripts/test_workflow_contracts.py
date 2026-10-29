import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOCKER_WORKFLOW = "build-docker.yml"


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
                self.assertNotIn("KASEKI_API_TOKEN:", caller)
        self.assertIn("if: github.ref == 'refs/heads/main'", reusable)
        self.assertIn("REF: main", reusable)
        self.assertIn('publishMode: "pr"', reusable)
        self.assertIn("environment: kaseki-agent", reusable)
        self.assertNotIn("KASEKI_API_TOKEN:", reusable.split("\njobs:", 1)[0])
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
        text = workflow(DOCKER_WORKFLOW)
        events = text.split("\npermissions:", 1)[0]
        self.assertIn("workflow_dispatch:", events)
        self.assertNotRegex(events, r"(?m)^  (?:push|schedule):")
        self.assertIn("image_version_tag:", events)
        self.assertIn("concurrency:", text)
        self.assertIn("sbom: true", text)
        self.assertIn("provenance: true", text)
        self.assertIn("needs: [validate, build, verify, scan]", text)
        self.assertIn("docker buildx imagetools create", text)
        self.assertIn("IMAGE_DIGEST", text)
        self.assertIn("validate_release_source.py", text)
        self.assertNotIn("validate_release_tag_event.py", text)
        self.assertNotIn("goreleaser/goreleaser-action", text)
        self.assertNotRegex(text, r"(?m)^  release:\n")
        self.assertRegex(text, r"(?m)^\s+timeout-minutes:")

    def test_docker_images_use_patched_go_and_upgrade_debian_base_packages(self):
        dockerfile = (ROOT / "Dockerfile").read_text()
        demo_dockerfile = (ROOT / "Dockerfile.demo").read_text()
        cloudbuild = (ROOT / "cloudbuild.yaml").read_text()
        go_mod = (ROOT / "go.mod").read_text()

        module_go_version = "1.27.2"
        docker_builder_go_version = "1.27.3"
        self.assertIn(
            f"FROM golang:{docker_builder_go_version}-alpine3.23 AS builder",
            dockerfile,
        )
        self.assertIn(
            f"FROM golang:{docker_builder_go_version}-alpine3.23 AS build",
            demo_dockerfile,
        )
        self.assertNotIn("GO_IMAGE=golang:", cloudbuild)
        self.assertNotIn("BUILDER_BASE_IMAGE=golang:", cloudbuild)
        self.assertIn(f"go {module_go_version}", go_mod)
        self.assertIn("apt-get upgrade -y --quiet --no-install-recommends", dockerfile)

    def test_docker_builds_cache_go_dependencies_and_build_outputs(self):
        dockerfile = (ROOT / "Dockerfile").read_text()
        demo_dockerfile = (ROOT / "Dockerfile.demo").read_text()

        for name, text in (("production", dockerfile), ("demo", demo_dockerfile)):
            with self.subTest(image=name):
                self.assertIn("COPY go.mod ./", text)
                self.assertIn("RUN go mod download", text)
                self.assertIn("target=/root/.cache/go-build", text)

        self.assertIn("FROM node:22-alpine AS web", demo_dockerfile)
        self.assertIn("COPY package.json package-lock.json", demo_dockerfile)
        self.assertIn("RUN npm ci", demo_dockerfile)
        self.assertIn("npm run build:web && npm run check:web", demo_dockerfile)
        self.assertIn("COPY --from=web /src/internal/web/index.html", demo_dockerfile)

        cloudbuild = (ROOT / "cloudbuild.yaml").read_text()
        self.assertIn("buildx build", cloudbuild)
        self.assertIn("type=registry,ref=", cloudbuild)
        self.assertIn("mode=max", cloudbuild)
        self.assertIn("--push", cloudbuild)

    def test_docker_images_are_pulled_and_scanned_per_architecture(self):
        text = workflow(DOCKER_WORKFLOW)
        build = workflow_step(text, "Build candidate and generate attestations")
        resolve = workflow_step(text, "Resolve candidate platform digests")
        scan = workflow_job(text, "scan")

        events = text.split("\npermissions:", 1)[0]
        self.assertNotIn("schedule:", events)
        self.assertIn("pull: true", build)
        self.assertNotIn("no-cache:", build)
        self.assertIn("verify_docker_manifest.py --github-output", resolve)
        self.assertIn("steps.platform-digests.outputs.amd64_digest", workflow_job(text, "build"))
        self.assertIn("steps.platform-digests.outputs.arm64_digest", workflow_job(text, "build"))
        self.assertIn("architecture: [amd64, arm64]", scan)
        self.assertIn("matrix.architecture == 'amd64'", scan)
        self.assertIn("version: v0.75.0", scan)

    def test_docker_image_version_tag_commit_is_on_main(self):
        text = workflow(DOCKER_WORKFLOW)
        validate = workflow_job(text, "validate")
        self.assertIn("if: inputs.image_version_tag != ''", validate)
        self.assertIn("validate_release_source.py", validate)
        self.assertIn("git fetch origin main", validate)
        self.assertIn(
            "steps.validate-source.outputs.source_sha || steps.manual-source.outputs.source_sha",
            validate,
        )
        self.assertIn(
            "ref: ${{ steps.validate-source.outputs.source_sha || steps.manual-source.outputs.source_sha }}",
            validate,
        )
        self.assertLess(validate.index("Checkout validated source"), validate.index("Run shared CI helper tests"))
        self.assertIn("ref: ${{ needs.validate.outputs.source_sha }}", workflow_job(text, "build"))
        self.assertNotRegex(text, r"(?m)^  release:\n")

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

    def test_manual_docker_publishes_serialize_and_combine_image_version_tags(self):
        text = workflow(DOCKER_WORKFLOW)
        self.assertIn(
            "group: docker-publish-${{ github.repository }}-manual",
            text,
        )
        self.assertIn('requested_tags="${IMAGE_VERSION_TAG},${requested_tags}"', text)
        self.assertNotIn('requested_tags="${GITHUB_REF_NAME},latest"', text)

    def test_pull_only_docker_jobs_use_read_only_credentials_and_scan_has_no_github_permissions(self):
        text = workflow(DOCKER_WORKFLOW)
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
        for name in ("code-coverage-test.yml", DOCKER_WORKFLOW):
            with self.subTest(workflow=name):
                self.assertIn("bash scripts/test-ci-helpers.sh", workflow(name))

        for name in (DOCKER_WORKFLOW,):
            with self.subTest(workflow=name):
                self.assertIn("bash scripts/test-go-ci.sh", workflow(name))

        for name in ("code-coverage-test.yml", DOCKER_WORKFLOW):
            with self.subTest(node_workflow=name):
                self.assertIn("actions/setup-node@", workflow(name))
                self.assertIn("node-version: 22", workflow(name))

        docker_build = workflow_job(workflow(DOCKER_WORKFLOW), "build")
        self.assertIn("npm run build:web", docker_build)
        self.assertLess(docker_build.index("npm run build:web"), docker_build.index("docker/build-push-action"))
        go_releaser = (ROOT / ".goreleaser.yml").read_text()
        self.assertIn("- cmd: npm run build:web", go_releaser)

        demo_dockerfile = (ROOT / "Dockerfile.demo").read_text()
        self.assertIn("npm run build:web", demo_dockerfile)
        self.assertIn("npm run check:web", demo_dockerfile)

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

    def test_kaseki_completion_checks_actual_changed_files_against_allowlist(self):
        reusable = workflow("kaseki-sweep.yml")
        wait = workflow_step(reusable, "Wait for Kaseki completion")
        self.assertIn("/api/v1/runs/$RUN_ID/analysis", wait)
        self.assertIn("scripts/validate_kaseki_changed_files.py", wait)

    def test_documentation_link_check_does_not_accept_http_errors_globally(self):
        text = workflow("documentation.yml")
        args = workflow_step(text, "Check canonical documentation links")
        self.assertIn("--accept 200..=299", args)
        self.assertNotIn("403", args)
        self.assertNotIn("429", args)
        self.assertIn(
            r"--exclude '^https://www\.raspberrypi\.com/documentation/computers/camera_software\.html'",
            args,
        )

if __name__ == "__main__":
    unittest.main()
