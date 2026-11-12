export type StreamState = "ready" | "connecting" | "live" | "reconnecting" | "unavailable";
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
    animationClass: "mio-blink" | "mio-pulse" | "mio-float";
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

const streamStatuses: Record<StreamState, StreamStatusPresentation> = {
    ready: { label: "Ready", tone: "neutral" },
    connecting: { label: "Connecting…", tone: "warning" },
    live: { label: "Live", tone: "success" },
    reconnecting: { label: "Reconnecting…", tone: "warning" },
    unavailable: { label: "Stream unavailable", tone: "error" },
};

export function getStreamStatusPresentation(state: StreamState): StreamStatusPresentation {
    return streamStatuses[state];
}

const streamMascots: Record<StreamState, StreamMascotPresentation> = {
    ready: {
        src: "/static/mio/mio_pose_sleeping.png?v=f7f7212e2ebdf8a6438d93e7d3e4c8a2a7c02a3278d393ba13aad2a7fe040d66",
        alt: "MIO is resting",
        animationClass: "mio-blink",
    },
    connecting: {
        src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
        alt: "MIO is curious",
        animationClass: "mio-pulse",
    },
    live: {
        src: "/static/mio/mio_pose_happy.png?v=e2bf5184a25fad3fdd17320965ecc8bd022126f26379ae7df2a857c71ee265be",
        alt: "MIO is happily watching",
        animationClass: "mio-float",
    },
    reconnecting: {
        src: "/static/mio/mio_pose_concerned.png?v=b28941a29aef2eebe3e74be347dfc9a6e208f61771ec72907e8e7700276f3b5c",
        alt: "MIO is concerned",
        animationClass: "mio-pulse",
    },
    unavailable: {
        src: "/static/mio/mio_pose_worried.png?v=2c136a60a29ff48447aa0bd6484050e7af02efa87cbcab7279db3455215f92e4",
        alt: "MIO is worried",
        animationClass: "mio-float",
    },
};

export function getStreamMascotPresentation(state: StreamState): StreamMascotPresentation {
    return streamMascots[state];
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

export function getStreamControlPresentation(state: StreamState): StreamControlPresentation {
    const active = state === "connecting" || state === "live" || state === "reconnecting";
    return {
        startHidden: active,
        startLabel: state === "unavailable" ? "Retry stream" : "Start stream",
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
