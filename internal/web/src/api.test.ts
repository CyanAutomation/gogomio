import assert from "node:assert/strict";
import test from "node:test";
import { requestCameraConfig, requestDiagnostics, requestLiveMetrics, requestStopStream } from "./api";

// Contract: TC-WEB-04 (docs/testing/test-contracts.md).
async function assertRequest(
    request: (fetcher: typeof fetch) => Promise<Response>,
    want: { url: string; method: string; cache?: RequestCache },
): Promise<void> {
    let observed: { url: string; method: string; cache?: RequestCache } | undefined;
    const fetcher: typeof fetch = async (input, init) => {
        observed = {
            url: String(input),
            method: init?.method ?? "GET",
            ...(init?.cache ? { cache: init.cache } : {}),
        };
        return new Response(null, { status: 204 });
    };

    await request(fetcher);
    assert.deepEqual(observed, want);
}

test("camera configuration uses the documented read endpoint", async () => {
    await assertRequest(requestCameraConfig, { url: "/v1/config/camera", method: "GET" });
});

test("live metrics use the no-store polling endpoint", async () => {
    await assertRequest(requestLiveMetrics, { url: "/v1/metrics/live", method: "GET", cache: "no-store" });
});

test("diagnostics use the documented read endpoint", async () => {
    await assertRequest(requestDiagnostics, { url: "/v1/health/detailed", method: "GET", cache: "no-store" });
});

test("stopping a stream uses the documented POST endpoint", async () => {
    await assertRequest(requestStopStream, { url: "/api/stream/stop", method: "POST" });
});
