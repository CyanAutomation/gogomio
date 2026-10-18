import assert from "node:assert/strict";
import test from "node:test";
import { requestCameraConfig, requestDiagnostics, requestStopStream } from "./api";

// Contract: TC-WEB-04 (docs/testing/test-contracts.md).
async function assertRequest(
    request: (fetcher: typeof fetch) => Promise<Response>,
    want: { url: string; method: string },
): Promise<void> {
    let observed: { url: string; method: string } | undefined;
    const fetcher: typeof fetch = async (input, init) => {
        observed = {
            url: String(input),
            method: init?.method ?? "GET",
        };
        return new Response(null, { status: 204 });
    };

    await request(fetcher);
    assert.deepEqual(observed, want);
}

test("camera configuration uses the documented read endpoint", async () => {
    await assertRequest(requestCameraConfig, { url: "/api/config", method: "GET" });
});

test("diagnostics use the documented read endpoint", async () => {
    await assertRequest(requestDiagnostics, { url: "/api/diagnostics", method: "GET" });
});

test("stopping a stream uses the documented POST endpoint", async () => {
    await assertRequest(requestStopStream, { url: "/api/stream/stop", method: "POST" });
});
