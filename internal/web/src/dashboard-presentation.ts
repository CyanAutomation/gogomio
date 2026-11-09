export type StreamState = "ready" | "connecting" | "live" | "reconnecting" | "unavailable";
export type StatusTone = "neutral" | "warning" | "success" | "error";
export type MetricTone = "neutral" | "success" | "warning" | "error";
export type CameraReadinessState = "checking" | "ready" | "initializing" | "unavailable" | "unknown";

export interface StreamStatusPresentation {
    label: string;
    tone: StatusTone;
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
    ready: { label: "Stream stopped", tone: "neutral" },
    connecting: { label: "Connecting…", tone: "warning" },
    live: { label: "Live", tone: "success" },
    reconnecting: { label: "Reconnecting…", tone: "warning" },
    unavailable: { label: "Stream unavailable", tone: "error" },
};

export function getStreamStatusPresentation(state: StreamState): StreamStatusPresentation {
    return streamStatuses[state];
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
