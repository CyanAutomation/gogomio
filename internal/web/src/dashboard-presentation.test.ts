import assert from "node:assert/strict";
import test from "node:test";

import { MIO_SPRITES } from "./mio-assets.generated";
import {
    ConsecutivePollHysteresis,
    MIO_SLEEP_AFTER_MS,
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getFrameRateTone,
    getCompanionPoseForFrameRateTone,
    getHealthTone,
    getCameraReadinessState,
    getCameraReadinessPresentation,
    getStreamMascotPresentation,
    getStreamControlPresentation,
    getStreamStatusPresentation,
    getStreamDescription,
    isCapacitySignal,
    isMioSleepDue,
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
    assert.deepEqual(getStreamStatusPresentation("capacity"), { label: "At capacity", tone: "warning" });
});

test("MIO uses a friendly pose for every stream state", () => {
    assert.deepEqual(getStreamMascotPresentation("ready"), {
        src: MIO_SPRITES.idle,
        alt: "MIO is ready",
        animationClass: "mio-float",
    });
    assert.deepEqual(getStreamMascotPresentation("connecting"), {
        src: MIO_SPRITES.curious,
        alt: "MIO is curious",
        animationClass: "mio-pulse",
    });
    assert.deepEqual(getStreamMascotPresentation("live"), {
        src: MIO_SPRITES.happy,
        alt: "MIO is happily watching",
        animationClass: "mio-float",
    });
    assert.deepEqual(getStreamMascotPresentation("reconnecting"), {
        src: MIO_SPRITES.concerned,
        alt: "MIO is concerned",
        animationClass: "mio-pulse",
    });
    assert.deepEqual(getStreamMascotPresentation("unavailable"), {
        src: MIO_SPRITES.worried,
        alt: "MIO is worried",
        animationClass: "mio-float",
    });
    assert.equal(getStreamMascotPresentation("capacity").src.split("?")[0], "/static/mio/mio_pose_angry.png");
});

test("stream descriptions follow the current state in MIO's voice", () => {
    assert.equal(getStreamDescription("ready"), "MIO's ready when you are.");
    assert.equal(getStreamDescription("connecting"), "MIO is warming things up…");
    assert.equal(getStreamDescription("live"), "MIO is happily watching your stream.");
    assert.equal(getStreamDescription("reconnecting"), "MIO is reconnecting (1 of 3)…");
    assert.equal(getStreamDescription("unavailable"), "MIO couldn't reconnect. Check the camera connection, then try again.");
    assert.equal(getStreamDescription("capacity"), "MIO's at capacity right now. Try again when another stream is free.");
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
    assert.deepEqual(getStreamControlPresentation("capacity"), {
        startHidden: false,
        startLabel: "Try again",
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

test("live frame-rate tones map to kind companion poses", () => {
    assert.equal(getCompanionPoseForFrameRateTone("success"), "happy");
    assert.equal(getCompanionPoseForFrameRateTone("warning"), "concerned");
    assert.equal(getCompanionPoseForFrameRateTone("error"), "worried");
});

test("pose changes require two consecutive polls and ignore a returned candidate", () => {
    const hysteresis = new ConsecutivePollHysteresis("happy", 2);

    assert.equal(hysteresis.update("concerned"), "happy");
    assert.equal(hysteresis.update("happy"), "happy");
    assert.equal(hysteresis.update("concerned"), "happy");
    assert.equal(hysteresis.update("concerned"), "concerned");
    assert.equal(hysteresis.update("worried"), "concerned");
    assert.equal(hysteresis.update("worried"), "worried");
});

test("MIO sleeps only after the named idle timeout", () => {
    assert.equal(MIO_SLEEP_AFTER_MS, 120_000);
    assert.equal(isMioSleepDue(10_000, 10_000 + MIO_SLEEP_AFTER_MS - 1), false);
    assert.equal(isMioSleepDue(10_000, 10_000 + MIO_SLEEP_AFTER_MS), true);
    assert.equal(isMioSleepDue(Number.NaN, 200_000), false);
});

test("capacity comes from a full server limit while idle or an HTTP 429", () => {
    assert.equal(isCapacitySignal(2, 2, false), true);
    assert.equal(isCapacitySignal(1, 2, false), false);
    assert.equal(isCapacitySignal(2, 2, true), false);
    assert.equal(isCapacitySignal(0, 2, false, 429), true);
    assert.equal(isCapacitySignal(0, 2, false, 503), false);
});

test("health presentation distinguishes idle, stale, failure, and healthy states", () => {
    assert.equal(getHealthTone("Idle", true, 0, 0), "neutral");
    assert.equal(getHealthTone("Degraded", true, 0, 0), "warning");
    assert.equal(getHealthTone("Poor", true, 0, 0), "error");
    assert.equal(getHealthTone("Excellent", true, 0, 0), "success");
    assert.equal(getHealthTone("Excellent", true, 0, 1), "warning");
    assert.equal(getHealthTone("Excellent", false, 0, 0), "error");
});
