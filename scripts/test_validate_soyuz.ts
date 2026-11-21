import assert from "node:assert/strict";
import test from "node:test";

import { validateSoyuzBaseUrl } from "./validate-soyuz-url.ts";
import { validateSoyuzRunId } from "./validate-soyuz-run-id.ts";

test("accepts the configured Soyuz origin with or without a trailing slash", () => {
  assert.equal(
    validateSoyuzBaseUrl("https://soyuz.scheimann.workers.dev"),
    "https://soyuz.scheimann.workers.dev",
  );
  assert.equal(
    validateSoyuzBaseUrl("https://soyuz.scheimann.workers.dev/"),
    "https://soyuz.scheimann.workers.dev",
  );
});

test("rejects malformed or unexpected Soyuz base URLs", () => {
  for (const value of [
    undefined,
    "",
    "https://soyuz.scheimann.workers.dev.evil.example",
    "http://soyuz.scheimann.workers.dev",
    "https://soyuz.scheimann.workers.dev/api",
    "https://user@soyuz.scheimann.workers.dev",
    "https://soyuz.scheimann.workers.dev?next=https://example.com",
    "https://soyuz.scheimann.workers.dev#fragment",
    "https://soyuz.scheimann.workers.dev ",
  ]) {
    assert.throws(() => validateSoyuzBaseUrl(value), String(value));
  }
});

test("accepts a Soyuz UUIDv7 run ID", () => {
  const runId = "0190f1c2-3d4e-7f50-8abc-123456789abc";
  assert.equal(validateSoyuzRunId(runId), runId);
});

test("rejects non-UUIDv7 run IDs", () => {
  for (const value of [
    undefined,
    "",
    "not-a-uuid",
    "0190f1c2-3d4e-4f50-8abc-123456789abc",
    "0190f1c2-3d4e-7f50-7abc-123456789abc",
  ]) {
    assert.throws(() => validateSoyuzRunId(value), String(value));
  }
});
