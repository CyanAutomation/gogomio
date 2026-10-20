export type StreamState = "ready" | "connecting" | "live" | "reconnecting" | "unavailable";
export type StatusTone = "neutral" | "warning" | "success" | "error";
export type MetricTone = "neutral" | "success" | "warning" | "error";

export interface StreamStatusPresentation {
    label: string;
    tone: StatusTone;
}

const streamStatuses: Record<StreamState, StreamStatusPresentation> = {
    ready: { label: "Ready", tone: "neutral" },
    connecting: { label: "Connecting…", tone: "warning" },
    live: { label: "Live", tone: "success" },
    reconnecting: { label: "Reconnecting…", tone: "warning" },
    unavailable: { label: "Unavailable", tone: "error" },
};

export function getStreamStatusPresentation(state: StreamState): StreamStatusPresentation {
    return streamStatuses[state];
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
