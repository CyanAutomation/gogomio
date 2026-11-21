import {
    isCapacitySignal,
    type MioPose,
} from "./dashboard-presentation";
import {
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getHealthTone,
} from "./metrics-presentation";
import {
    renderIcon,
    renderMetricCard,
    renderMetricRow,
    renderNotice,
    renderSectionHeading,
    renderStatusMetricCard,
} from "./ui-components";

export interface DiagnosticsResponse {
    camera_ready: boolean;
    camera_backend?: string;
    capture_failures_consecutive: number;
    capture_failures_total: number;
    error_rate_percent: number;
    stream_connections: number;
    max_stream_connections: number;
    health_status: string;
    uptime_seconds: number;
    fps_current: number;
    resolution: string;
    jpeg_quality: number;
    frames_captured: number;
    last_frame_age_seconds?: number | null;
    message: string;
}

export function getDiagnosticsCompanionPose(data: DiagnosticsResponse): MioPose | "capacity" {
    const consecutiveFailures = Number(data.capture_failures_consecutive || 0);
    const errorRate = Number(data.error_rate_percent || 0);
    const connections = Number(data.stream_connections || 0);

    if (isCapacitySignal(data.stream_connections, data.max_stream_connections, false)) return "capacity";
    if (!data.camera_ready && consecutiveFailures > 3) return "worried";
    if (!data.camera_ready || errorRate > 20 || consecutiveFailures > 0) return "concerned";
    if (connections > 0) return "curious";
    if (errorRate > 5) return "concerned";
    return "happy";
}

export function renderDiagnosticsContent(data: DiagnosticsResponse): string {
    return `
        ${renderSectionHeading("Camera health", "camera")}
        ${renderDiagnosticsSummary(data)}
        ${renderDiagnosticsDetails(data)}
        <div class="refresh-indicator">${renderIcon("refresh")}<span>Refreshes every 3 seconds</span></div>
    `;
}

function renderDiagnosticsSummary(data: DiagnosticsResponse): string {
    const healthTone = getHealthTone(
        data.health_status,
        data.camera_ready,
        Number(data.error_rate_percent || 0),
        Number(data.capture_failures_consecutive || 0),
    );
    const fallback = data.camera_backend === "mock-fallback";
    const mock = data.camera_backend === "mock";
    const cameraTone = !data.camera_ready ? "error" : fallback || mock ? "warning" : "success";
    const cameraStatus = !data.camera_ready ? "Unavailable" : fallback ? "Simulated fallback" : mock ? "Test camera" : "Ready";
    const noticeTone = healthTone === "neutral" ? "info" : healthTone;

    return `
        <div class="diagnostic-summary-grid">
            ${renderStatusMetricCard("Camera", cameraStatus, cameraTone)}
            ${renderStatusMetricCard("System health", data.health_status || "Unknown", healthTone)}
        </div>
        ${renderNotice(noticeTone, "System status", data.message || "No status details right now.", noticeTone === "error" ? "alert" : "info")}
    `;
}

function renderDiagnosticsDetails(data: DiagnosticsResponse): string {
    const errorRate = Number(data.error_rate_percent || 0);
    const consecutiveFailures = Number(data.capture_failures_consecutive || 0);
    const healthTone = getHealthTone(data.health_status, data.camera_ready, errorRate, consecutiveFailures);
    const quality = Number(data.jpeg_quality || 0);
    const framesCaptured = Number(data.frames_captured || 0);
    const connections = Number(data.stream_connections || 0);
    const maximumConnections = Number(data.max_stream_connections || 0);
    const totalFailures = Number(data.capture_failures_total || 0);

    return `
        <details class="diagnostics-details">
            <summary class="disclosure-summary">Detailed metrics</summary>
            <div class="diagnostics-secondary-grid">
                ${renderMetricCard("Uptime", formatUptime(Number(data.uptime_seconds || 0)))}
                ${renderMetricCard("Error rate", `${errorRate.toFixed(1)}%`, healthTone)}
            </div>

            <div class="diagnostic-metric-list">
                ${renderMetricRow("Frame rate", formatFrameRate(Number(data.fps_current ?? Number.NaN)), "camera")}
                ${data.camera_backend ? renderMetricRow("Camera source", data.camera_backend, "camera") : ""}
                ${renderMetricRow("Resolution", formatResolution(data.resolution), "grid")}
                ${renderMetricRow("JPEG quality", `${quality}%`, "quality")}
                ${renderMetricRow("Frames captured", framesCaptured.toLocaleString(), "image")}
                ${renderMetricRow("Last frame age", formatFrameAge(data.last_frame_age_seconds), "clock")}
                ${renderMetricRow("Active streams", formatStreamConnections(connections, maximumConnections), "link")}
                ${renderMetricRow("Recent failures", consecutiveFailures, "alert", consecutiveFailures > 0 ? "error" : "neutral")}
                ${renderMetricRow("Total failures", totalFailures, "chart")}
            </div>
        </details>
    `;
}

function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    const secs = Math.floor(seconds % 60);

    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    if (mins > 0) return `${mins}m ${secs}s`;
    return `${secs}s`;
}
