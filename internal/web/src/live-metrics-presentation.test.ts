import assert from "node:assert/strict";
import test from "node:test";

import {
    getLiveMetricsPresentation,
    type CameraConfigResponse,
    type LiveMetricsResponse,
} from "./live-metrics-presentation";

const config: CameraConfigResponse = {
    resolution: [1280, 720],
    fps: 24,
    target_fps: 24,
    jpeg_quality: 85,
    mock_camera: true,
    max_stream_connections: 2,
};

const metrics: LiveMetricsResponse = {
    fps_current: 21,
    stream_connections: 1,
};

test("live metrics presentation combines configuration and live values", () => {
    assert.deepEqual(getLiveMetricsPresentation(config, metrics, true), {
        aspectRatio: "1280 / 720",
        feedModeHidden: false,
        fpsText: "21.0 fps",
        fpsTone: "warning",
        resolutionText: "1280 × 720",
        qualityText: "85%",
        connectionsText: "1 / 2",
        capacitySignal: false,
    });
});

test("idle metrics hide the frame rate and detect full stream capacity", () => {
    assert.deepEqual(getLiveMetricsPresentation(config, {
        ...metrics,
        stream_connections: 2,
    }, false), {
        aspectRatio: "1280 / 720",
        feedModeHidden: false,
        fpsText: "",
        fpsTone: "neutral",
        resolutionText: "1280 × 720",
        qualityText: "85%",
        connectionsText: "2 / 2",
        capacitySignal: true,
    });
});

test("missing target frame rate keeps the tone neutral and camera feed mode hidden", () => {
    assert.deepEqual(getLiveMetricsPresentation({
        ...config,
        target_fps: null,
        mock_camera: false,
    }, metrics, true), {
        aspectRatio: "1280 / 720",
        feedModeHidden: true,
        fpsText: "21.0 fps",
        fpsTone: "neutral",
        resolutionText: "1280 × 720",
        qualityText: "85%",
        connectionsText: "1 / 2",
        capacitySignal: false,
    });
});

test("a zero JPEG quality is rendered as configured rather than missing", () => {
    const presentation = getLiveMetricsPresentation({ ...config, jpeg_quality: 0 }, metrics, true);
    assert.equal(presentation.qualityText, "0%");
});
