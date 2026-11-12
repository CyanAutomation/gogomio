import { MIO_SPRITES } from "./mio-assets.generated";

export type StreamState = "ready" | "connecting" | "live" | "reconnecting" | "unavailable" | "capacity";
export type MioPose = "idle" | "sleeping" | "curious" | "concerned" | "happy" | "worried" | "angry" | "looking";
export type StatusTone = "neutral" | "warning" | "success" | "error";
export type MetricTone = "neutral" | "success" | "warning" | "error";
export type CameraReadinessState = "checking" | "ready" | "initializing" | "unavailable" | "unknown";

export interface StreamStatusPresentation {
    label: string;
    tone: StatusTone;
}

export interface StreamMascotPresentation {
    src: string;
    alt: string;
    animationClass: "mio-pulse" | "mio-float";
}

export interface StreamControlPresentation {
    startHidden: boolean;
    startLabel: string;
    stopHidden: boolean;
    stopLabel: string;
    stopPrimary: boolean;
}

export interface CameraReadinessPresentation {
    label: string;
    tone: "info" | MetricTone;
    guidance: string;
}

export const MIO_SLEEP_AFTER_MS = 2 * 60 * 1000;

const streamStatuses: Record<StreamState, StreamStatusPresentation> = {
    ready: { label: "Ready", tone: "neutral" },
    connecting: { label: "Connecting…", tone: "warning" },
    live: { label: "Live", tone: "success" },
    reconnecting: { label: "Reconnecting…", tone: "warning" },
    unavailable: { label: "Unavailable", tone: "error" },
    capacity: { label: "At capacity", tone: "warning" },
};

export function getStreamStatusPresentation(state: StreamState): StreamStatusPresentation {
    return streamStatuses[state];
}

export function getStreamDescription(state: StreamState, retryCount = 1, maxRetries = 3): string {
    switch (state) {
        case "ready": return "MIO's ready when you are.";
        case "connecting": return "MIO is warming things up…";
        case "live": return "MIO is happily watching your stream.";
        case "reconnecting": return `MIO is reconnecting (${Math.max(1, retryCount)} of ${maxRetries})…`;
        case "unavailable": return "MIO couldn't reconnect. Check the camera connection, then try again.";
        case "capacity": return "MIO's at capacity right now. Try again when another stream is free.";
    }
}

const mioPoses: Record<MioPose, StreamMascotPresentation> = {
    idle: { src: MIO_SPRITES.idle, alt: "MIO is ready", animationClass: "mio-float" },
    sleeping: { src: MIO_SPRITES.sleeping, alt: "MIO is sleeping", animationClass: "mio-float" },
    curious: { src: MIO_SPRITES.curious, alt: "MIO is curious", animationClass: "mio-pulse" },
    concerned: { src: MIO_SPRITES.concerned, alt: "MIO is concerned", animationClass: "mio-pulse" },
    happy: { src: MIO_SPRITES.happy, alt: "MIO is happily watching", animationClass: "mio-float" },
    worried: { src: MIO_SPRITES.worried, alt: "MIO is worried", animationClass: "mio-float" },
    angry: { src: MIO_SPRITES.angry, alt: "MIO is looking out for stream capacity", animationClass: "mio-float" },
    looking: { src: MIO_SPRITES.looking, alt: "MIO is scanning", animationClass: "mio-float" },
};

const streamPoses: Record<StreamState, MioPose> = {
    ready: "idle",
    connecting: "curious",
    live: "happy",
    reconnecting: "concerned",
    unavailable: "worried",
    capacity: "angry",
};

export function getMioPosePresentation(pose: MioPose): StreamMascotPresentation {
    return mioPoses[pose];
}

export function getStreamMascotPresentation(state: StreamState): StreamMascotPresentation {
    return getMioPosePresentation(streamPoses[state]);
}

export function getCompanionPoseForFrameRateTone(tone: MetricTone): "happy" | "concerned" | "worried" {
    switch (tone) {
        case "success":
        case "neutral":
            return "happy";
        case "warning":
            return "concerned";
        case "error":
            return "worried";
    }
}

export class ConsecutivePollHysteresis<T> {
    private candidate: T | null = null;
    private consecutiveCount = 0;
    private currentValue: T;

    constructor(initialValue: T, private readonly minimumConsecutive = 2) {
        if (!Number.isInteger(minimumConsecutive) || minimumConsecutive < 1) {
            throw new RangeError("minimumConsecutive must be a positive integer");
        }
        this.currentValue = initialValue;
    }

    get value(): T {
        return this.currentValue;
    }

    update(nextValue: T): T {
        if (nextValue === this.currentValue) {
            this.candidate = null;
            this.consecutiveCount = 0;
            return this.currentValue;
        }

        if (nextValue === this.candidate) {
            this.consecutiveCount += 1;
        } else {
            this.candidate = nextValue;
            this.consecutiveCount = 1;
        }

        if (this.consecutiveCount >= this.minimumConsecutive) {
            this.currentValue = nextValue;
            this.candidate = null;
            this.consecutiveCount = 0;
        }

        return this.currentValue;
    }

    reset(value: T): void {
        this.currentValue = value;
        this.candidate = null;
        this.consecutiveCount = 0;
    }
}

export function isMioSleepDue(idleSince: number, now: number): boolean {
    return Number.isFinite(idleSince) && Number.isFinite(now) && now - idleSince >= MIO_SLEEP_AFTER_MS;
}

export function isCapacitySignal(
    activeConnections: number,
    maximumConnections: number,
    isStreaming: boolean,
    responseStatus = 0,
): boolean {
    if (responseStatus === 429) return true;
    return !isStreaming
        && Number.isFinite(activeConnections)
        && Number.isFinite(maximumConnections)
        && maximumConnections > 0
        && activeConnections >= maximumConnections;
}

export function getCameraReadinessPresentation(state: CameraReadinessState): CameraReadinessPresentation {
    const presentations: Record<CameraReadinessState, CameraReadinessPresentation> = {
        checking: { label: "Checking camera…", tone: "info", guidance: "" },
        ready: { label: "Camera ready", tone: "success", guidance: "" },
        initializing: {
            label: "Camera starting",
            tone: "warning",
            guidance: "The camera is still starting. Try again in a moment.",
        },
        unavailable: {
            label: "Camera unavailable",
            tone: "error",
            guidance: "Check the camera connection, then try the stream again.",
        },
        unknown: {
            label: "Camera status unknown",
            tone: "warning",
            guidance: "Could not check camera status. Check your connection to GoGoMio.",
        },
    };

    return presentations[state];
}

export function getCameraReadinessState(
    responseOk: boolean,
    status: string | undefined,
): "ready" | "initializing" | "unavailable" {
    if (!responseOk) return "unavailable";
    if (status === "ready" || status === "initializing") return status;
    return "unavailable";
}

export function getStreamControlPresentation(state: StreamState): StreamControlPresentation {
    const active = state === "connecting" || state === "live" || state === "reconnecting";
    return {
        startHidden: active,
        startLabel: state === "unavailable" ? "Retry stream" : state === "capacity" ? "Try again" : "Start stream",
        stopHidden: !active,
        stopLabel: state === "connecting" ? "Cancel connection" : "Stop stream",
        stopPrimary: state === "live" || state === "reconnecting",
    };
}

export function formatFrameRate(frameRate: number): string {
    return Number.isFinite(frameRate) && frameRate >= 0 ? `${frameRate.toFixed(1)} fps` : "--";
}

export function getFrameRateTone(frameRate: number, targetFrameRate: number, isLive: boolean): MetricTone {
    if (!isLive || !Number.isFinite(frameRate) || !Number.isFinite(targetFrameRate) || targetFrameRate <= 0) {
        return "neutral";
    }

    const ratio = frameRate / targetFrameRate;
    return ratio >= 0.9 ? "success" : ratio >= 0.5 ? "warning" : "error";
}

export function getHealthTone(
    healthStatus: string,
    cameraReady: boolean,
    errorRate: number,
    consecutiveFailures: number,
): MetricTone {
    if (!cameraReady || healthStatus === "Poor" || errorRate > 20) return "error";
    if (healthStatus === "Degraded" || errorRate > 5 || consecutiveFailures > 0) return "warning";
    if (healthStatus === "Idle") return "neutral";
    return "success";
}

export function formatResolution(resolution: [number, number] | string | null | undefined): string {
    if (Array.isArray(resolution)) {
        const [width, height] = resolution;
        return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
            ? `${width} × ${height}`
            : "--";
    }

    if (typeof resolution !== "string") return "--";
    const match = resolution.match(/^\s*(\d+)\s*[x×]\s*(\d+)\s*$/i);
    return match ? `${match[1]} × ${match[2]}` : "--";
}

export function formatStreamConnections(active: number, maximum: number): string {
    const activeText = Number.isFinite(active) && active >= 0 ? String(active) : "--";
    const maximumText = Number.isFinite(maximum) && maximum >= 0 ? String(maximum) : "--";
    return `${activeText} / ${maximumText}`;
}

export function formatFrameAge(ageSeconds: unknown): string {
    return typeof ageSeconds === "number" && Number.isFinite(ageSeconds) && ageSeconds >= 0
        ? `${ageSeconds.toFixed(2)}s ago`
        : "No frames yet";
}
