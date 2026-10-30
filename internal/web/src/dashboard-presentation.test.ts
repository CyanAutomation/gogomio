import assert from "node:assert/strict";
import test from "node:test";

import {
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getFrameRateTone,
    getHealthTone,
    getCameraOverviewPresentation,
    getStreamControlPresentation,
    getStreamStatusPresentation,
} from "./dashboard-presentation";

test("frame rate shows the measured rate with a unit and handles missing data", () => {
    assert.equal(formatFrameRate(0), "0.0 fps");
    assert.equal(formatFrameRate(23.84), "23.8 fps");
    assert.equal(formatFrameRate(Number.NaN), "--");
});

test("resolution uses one readable format for configuration and diagnostics", () => {
    assert.equal(formatResolution([640, 480]), "640 × 480");
    assert.equal(formatResolution("640x480"), "640 × 480");
    assert.equal(formatResolution("640 × 480"), "640 × 480");
    assert.equal(formatResolution("unknown"), "--");
});

test("stream connections show active and available capacity together", () => {
    assert.equal(formatStreamConnections(0, 2), "0 / 2");
    assert.equal(formatStreamConnections(1, 2), "1 / 2");
    assert.equal(formatStreamConnections(Number.NaN, 2), "-- / 2");
});

test("an unavailable frame age is described without exposing its negative sentinel", () => {
    assert.equal(formatFrameAge(-1), "No frames yet");
    assert.equal(formatFrameAge(Number.NaN), "No frames yet");
    assert.equal(formatFrameAge(undefined), "No frames yet");
    assert.equal(formatFrameAge(null), "No frames yet");
    assert.equal(formatFrameAge(0.034), "0.03s ago");
});

test("stream states have consistent labels and semantic tones", () => {
    assert.deepEqual(getStreamStatusPresentation("ready"), { label: "Ready", tone: "neutral" });
    assert.deepEqual(getStreamStatusPresentation("connecting"), { label: "Connecting…", tone: "warning" });
    assert.deepEqual(getStreamStatusPresentation("live"), { label: "Live", tone: "success" });
    assert.deepEqual(getStreamStatusPresentation("reconnecting"), { label: "Reconnecting…", tone: "warning" });
    assert.deepEqual(getStreamStatusPresentation("unavailable"), { label: "Unavailable", tone: "error" });
});

test("camera overview gives users next-step context for each stream state", () => {
    assert.deepEqual(getCameraOverviewPresentation("ready"), {
        title: "Ready to start",
        description: "Start the stream to see live camera performance.",
    });
    assert.deepEqual(getCameraOverviewPresentation("connecting"), {
        title: "Connecting to camera",
        description: "Live camera performance will appear when frames arrive.",
    });
    assert.deepEqual(getCameraOverviewPresentation("live"), {
        title: "Streaming live",
        description: "The frame rate is shown here. Open camera details for the rest.",
    });
    assert.deepEqual(getCameraOverviewPresentation("reconnecting"), {
        title: "Reconnecting",
        description: "The stream paused. GoGoMio is trying to restore it.",
    });
    assert.deepEqual(getCameraOverviewPresentation("unavailable"), {
        title: "Stream unavailable",
        description: "Check the camera connection, then try again.",
    });
});

test("only the action relevant to the current stream state is shown", () => {
    assert.deepEqual(getStreamControlPresentation("ready"), {
        startHidden: false,
        stopHidden: true,
        stopPrimary: false,
    });
    assert.deepEqual(getStreamControlPresentation("connecting"), {
        startHidden: true,
        stopHidden: false,
        stopPrimary: true,
    });
    assert.deepEqual(getStreamControlPresentation("live"), {
        startHidden: true,
        stopHidden: false,
        stopPrimary: true,
    });
    assert.deepEqual(getStreamControlPresentation("reconnecting"), {
        startHidden: true,
        stopHidden: false,
        stopPrimary: true,
    });
    assert.deepEqual(getStreamControlPresentation("unavailable"), {
        startHidden: false,
        stopHidden: true,
        stopPrimary: false,
    });
});

test("frame rate color is neutral while idle and compares live rate to target", () => {
    assert.equal(getFrameRateTone(0, 24, false), "neutral");
    assert.equal(getFrameRateTone(23, 24, true), "success");
    assert.equal(getFrameRateTone(18, 24, true), "warning");
    assert.equal(getFrameRateTone(5, 24, true), "error");
    assert.equal(getFrameRateTone(10, 0, true), "neutral");
    assert.equal(getFrameRateTone(10, Number.NaN, true), "neutral");
});

test("health presentation distinguishes idle, stale, failure, and healthy states", () => {
    assert.equal(getHealthTone("Idle", true, 0, 0), "neutral");
    assert.equal(getHealthTone("Degraded", true, 0, 0), "warning");
    assert.equal(getHealthTone("Poor", true, 0, 0), "error");
    assert.equal(getHealthTone("Excellent", true, 0, 0), "success");
    assert.equal(getHealthTone("Excellent", true, 0, 1), "warning");
    assert.equal(getHealthTone("Excellent", false, 0, 0), "error");
});
