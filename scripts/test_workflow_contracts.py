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

        self.assertIn("golang:1.25.13-alpine3.23", dockerfile)
        self.assertIn("golang:1.25.13-alpine3.23", demo_dockerfile)
        self.assertIn("GO_IMAGE=golang:1.25.13-alpine3.23", cloudbuild)
        self.assertNotIn("BUILDER_BASE_IMAGE=golang:", cloudbuild)
        self.assertIn("go 1.25.13", go_mod)
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
        for name in ("benchmark.yml", "code-coverage-test.yml"):
            with self.subTest(workflow=name):
                self.assertGreaterEqual(workflow(name).count("persist-credentials: false"), 1)
        benchmark = workflow("benchmark.yml")
        history = workflow_job(benchmark, "history")
        self.assertIn("GH_TOKEN: ${{ github.token }}", history)

    def test_pr_ci_cancels_obsolete_runs_without_cancelling_other_events(self):
        for name in ("benchmark.yml", "code-coverage-test.yml"):
            with self.subTest(workflow=name):
                text = workflow(name)
                self.assertIn(
                    "group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}-${{ github.event_name }}",
                    text,
                )
                self.assertIn(
                    "cancel-in-progress: ${{ github.event_name == 'pull_request' }}",
                    text,
                )

    def test_benchmark_history_api_permission_is_scoped_to_trusted_job(self):
        text = workflow("benchmark.yml")
        top_level = text.split("jobs:\n", 1)[0]
        self.assertIn("contents: read", top_level)
        self.assertNotIn("actions: read", top_level)

        history = workflow_job(text, "history")
        benchmark = workflow_job(text, "benchmark")
        self.assertIn(
            "if: github.event_name == 'schedule' || (github.event_name == 'workflow_dispatch' && github.ref == 'refs/heads/main')",
            history,
        )
        self.assertIn("actions: read", history)
        self.assertNotIn("contents: read", history)
        self.assertIn("GH_TOKEN: ${{ github.token }}", history)
        self.assertIn("actions/download-artifact@", benchmark)
        self.assertNotIn("GH_TOKEN:", benchmark)
        self.assertIn("needs: history", benchmark)
        self.assertIn("github.event_name != 'workflow_dispatch' || github.ref == 'refs/heads/main'", benchmark)

    def test_benchmark_uses_shorter_pr_samples_and_a_larger_full_run_budget(self):
        text = workflow("benchmark.yml")
        self.assertIn("timeout-minutes: 30", text)
        self.assertIn("BENCH_COUNT: ${{ github.event_name == 'pull_request' && '5' || '10' }}", text)
        self.assertGreaterEqual(text.count('-count="$BENCH_COUNT"'), 2)

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

    def test_shared_ci_helpers_and_actionlint_run_in_workflows(self):
        for name in ("code-coverage-test.yml", "build-multiarch.yml"):
            with self.subTest(workflow=name):
                self.assertIn("bash scripts/test-ci-helpers.sh", workflow(name))

        for name in ("build-multiarch.yml", "goreleaser.yml"):
            with self.subTest(workflow=name):
                self.assertIn("bash scripts/test-go-ci.sh", workflow(name))
        self.assertIn(
            "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.11 -color",
            workflow("code-coverage-test.yml"),
        )
        helper_tests = (ROOT / "scripts" / "test-ci-helpers.sh").read_text()
        go_tests = (ROOT / "scripts" / "test-go-ci.sh").read_text()
        self.assertIn("python3 scripts/check_skills.py", helper_tests)
        self.assertIn("python3 -B -m unittest discover -s scripts -p 'test_*.py'", helper_tests)
        self.assertIn("go vet ./...", go_tests)
        self.assertIn("go test ./... -race", go_tests)

    def test_workflow_shellcheck_warnings_are_avoided(self):
        benchmark = workflow_step(workflow("benchmark.yml"), "Prepare comparison baseline")
        self.assertIn('} >> "$GITHUB_OUTPUT"', benchmark)
        self.assertNotIn("echo 'available=true' >> \"$GITHUB_OUTPUT\"", benchmark)
        self.assertNotIn('echo "available=true" >> "$GITHUB_OUTPUT"', benchmark)

        gofmt = workflow_step(workflow("code-coverage-test.yml"), "Check gofmt")
        self.assertIn("mapfile -t go_files < <(git ls-files '*.go')", gofmt)
        self.assertIn('gofmt -l "${go_files[@]}"', gofmt)
        self.assertNotIn("$(git ls-files", gofmt)

    def test_coverage_threshold_uses_strict_shell_and_rejects_unparseable_output(self):
        step = workflow_step(workflow("code-coverage-test.yml"), "Enforce coverage threshold (≥75%)")
        self.assertIn("set -euo pipefail", step)
        self.assertIn("Could not parse total coverage", step)

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
                self.assertIn("bash scripts/test-ci-helpers.sh", workflow(name))
        self.assertIn(command, (ROOT / "scripts" / "test-ci-helpers.sh").read_text())


if __name__ == "__main__":
    unittest.main()
