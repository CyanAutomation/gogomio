import assert from "node:assert/strict";
import test from "node:test";

import {
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getFrameRateTone,
    getHealthTone,
    getCameraReadinessState,
    getCameraReadinessPresentation,
    getStreamMascotPresentation,
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
    assert.deepEqual(getStreamStatusPresentation("unavailable"), { label: "Stream unavailable", tone: "error" });
});

test("MIO uses a friendly pose for every stream state", () => {
    assert.deepEqual(getStreamMascotPresentation("ready"), {
        src: "/static/mio/mio_pose_idle.png?v=fa5479ac7c65b12baf9cc84043969af4b75b8fd7a99a92b30e29d6961b016c6b",
        alt: "MIO is ready",
        animationClass: "mio-float",
    });
    assert.deepEqual(getStreamMascotPresentation("connecting"), {
        src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
        alt: "MIO is curious",
        animationClass: "mio-pulse",
    });
    assert.deepEqual(getStreamMascotPresentation("live"), {
        src: "/static/mio/mio_pose_happy.png?v=e2bf5184a25fad3fdd17320965ecc8bd022126f26379ae7df2a857c71ee265be",
        alt: "MIO is happily watching",
        animationClass: "mio-float",
    });
    assert.deepEqual(getStreamMascotPresentation("reconnecting"), {
        src: "/static/mio/mio_pose_concerned.png?v=b28941a29aef2eebe3e74be347dfc9a6e208f61771ec72907e8e7700276f3b5c",
        alt: "MIO is concerned",
        animationClass: "mio-pulse",
    });
    assert.deepEqual(getStreamMascotPresentation("unavailable"), {
        src: "/static/mio/mio_pose_worried.png?v=2c136a60a29ff48447aa0bd6484050e7af02efa87cbcab7279db3455215f92e4",
        alt: "MIO is worried",
        animationClass: "mio-float",
    });
});

test("camera readiness has distinct copy and tones for checking, ready, starting, and unavailable", () => {
    assert.deepEqual(getCameraReadinessPresentation("checking"), {
        label: "Checking camera…",
        tone: "info",
        guidance: "",
    });
    assert.deepEqual(getCameraReadinessPresentation("ready"), {
        label: "Camera ready",
        tone: "success",
        guidance: "",
    });
    assert.deepEqual(getCameraReadinessPresentation("initializing"), {
        label: "Camera starting",
        tone: "warning",
        guidance: "The camera is still starting. Try again in a moment.",
    });
    assert.deepEqual(getCameraReadinessPresentation("unavailable"), {
        label: "Camera unavailable",
        tone: "error",
        guidance: "Check the camera connection, then try the stream again.",
    });
    assert.deepEqual(getCameraReadinessPresentation("unknown"), {
        label: "Camera status unknown",
        tone: "warning",
        guidance: "Could not check camera status. Check your connection to GoGoMio.",
    });
});

test("camera readiness treats non-success responses as unavailable regardless of the body status", () => {
    assert.equal(getCameraReadinessState(false, "initializing"), "unavailable");
    assert.equal(getCameraReadinessState(false, "ready"), "unavailable");
    assert.equal(getCameraReadinessState(true, "initializing"), "initializing");
    assert.equal(getCameraReadinessState(true, "ready"), "ready");
    assert.equal(getCameraReadinessState(true, "unexpected"), "unavailable");
});

test("only the action relevant to the current stream state is shown", () => {
    assert.deepEqual(getStreamControlPresentation("ready"), {
        startHidden: false,
        startLabel: "Start stream",
        stopHidden: true,
        stopLabel: "Stop stream",
        stopPrimary: false,
    });
    assert.deepEqual(getStreamControlPresentation("connecting"), {
        startHidden: true,
        startLabel: "Start stream",
        stopHidden: false,
        stopLabel: "Cancel connection",
        stopPrimary: false,
    });
    assert.deepEqual(getStreamControlPresentation("live"), {
        startHidden: true,
        startLabel: "Start stream",
        stopHidden: false,
        stopLabel: "Stop stream",
        stopPrimary: true,
    });
    assert.deepEqual(getStreamControlPresentation("reconnecting"), {
        startHidden: true,
        startLabel: "Start stream",
        stopHidden: false,
        stopLabel: "Stop stream",
        stopPrimary: true,
    });
    assert.deepEqual(getStreamControlPresentation("unavailable"), {
        startHidden: false,
        startLabel: "Retry stream",
        stopHidden: true,
        stopLabel: "Stop stream",
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
