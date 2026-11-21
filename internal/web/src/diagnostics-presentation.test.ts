import assert from "node:assert/strict";
import test from "node:test";

import {
    getDiagnosticsCompanionPose,
    renderDiagnosticsContent,
    type DiagnosticsResponse,
} from "./diagnostics-presentation";

const healthyDiagnostics: DiagnosticsResponse = {
    camera_ready: true,
    capture_failures_consecutive: 0,
    capture_failures_total: 12,
    error_rate_percent: 0,
    stream_connections: 0,
    max_stream_connections: 2,
    health_status: "Healthy",
    uptime_seconds: 3665,
    fps_current: 24.4,
    resolution: "640x480",
    jpeg_quality: 80,
    frames_captured: 1200,
    last_frame_age_seconds: 0.034,
    message: "Camera ready",
};

test("diagnostics companion prioritizes full stream capacity", () => {
    assert.equal(getDiagnosticsCompanionPose({
        ...healthyDiagnostics,
        camera_ready: false,
        capture_failures_consecutive: 8,
        stream_connections: 2,
    }), "capacity");
});

test("diagnostics companion distinguishes repeated camera failures from other warnings", () => {
    assert.equal(getDiagnosticsCompanionPose({
        ...healthyDiagnostics,
        camera_ready: false,
        capture_failures_consecutive: 4,
    }), "worried");
    assert.equal(getDiagnosticsCompanionPose({
        ...healthyDiagnostics,
        camera_ready: false,
        capture_failures_consecutive: 3,
    }), "concerned");
});

test("diagnostics companion follows the established warning and connection priority", () => {
    assert.equal(getDiagnosticsCompanionPose({
        ...healthyDiagnostics,
        error_rate_percent: 21,
        stream_connections: 1,
    }), "concerned");
    assert.equal(getDiagnosticsCompanionPose({
        ...healthyDiagnostics,
        error_rate_percent: 6,
        stream_connections: 1,
    }), "curious");
    assert.equal(getDiagnosticsCompanionPose({ ...healthyDiagnostics, error_rate_percent: 6 }), "concerned");
    assert.equal(getDiagnosticsCompanionPose(healthyDiagnostics), "happy");
});

test("diagnostics content formats metrics and escapes server-provided copy", () => {
    const html = renderDiagnosticsContent({
        ...healthyDiagnostics,
        message: '<img src=x onerror="alert(1)">',
    });

    assert.match(html, /Camera health/);
    assert.match(html, /1h 1m/);
    assert.match(html, /24\.4 fps/);
    assert.match(html, /640 × 480/);
    assert.match(html, /1,200/);
    assert.match(html, /0\.03s ago/);
    assert.match(html, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
    assert.doesNotMatch(html, /<img src=x/);
});

test("diagnostics identify simulated fallback as the active camera source", () => {
    const html = renderDiagnosticsContent({
        ...healthyDiagnostics,
        camera_backend: "mock-fallback",
        health_status: "Degraded",
        message: "Real camera startup failed; simulated frames are active while retrying.",
    });

    assert.match(html, />Simulated fallback</);
    assert.match(html, /Camera source[\s\S]*mock-fallback/);
    assert.match(html, /Real camera startup failed/);
});

test("diagnostics render uptime with the largest complete unit", () => {
    for (const [seconds, expected] of [[0, "0s"], [65, "1m 5s"], [65.9, "1m 5s"], [3605, "1h 0m"], [86461, "1d 0h"]] as const) {
        assert.ok(renderDiagnosticsContent({ ...healthyDiagnostics, uptime_seconds: seconds }).includes(expected));
    }
});
