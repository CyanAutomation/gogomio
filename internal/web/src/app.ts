import { resolutionToAspectRatio } from "./aspect-ratio";
import { requestCameraConfig, requestDiagnostics, requestLiveMetrics, requestStopStream } from "./api";
import {
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getFrameRateTone,
    getHealthTone,
    getStreamStatusPresentation,
    type StreamState,
} from "./dashboard-presentation";
import {
    handleKeydown,
    rememberReturnFocus,
    restoreReturnFocus,
    type ReturnFocusTarget,
} from "./diagnostics-dialog";
import { nextStreamRetryCount, shouldRunStreamRetry } from "./stream-retry";
import { shouldRunDashboardPoller } from "./polling";

type PoseState = "idle" | "loading" | "success" | "warning" | "running" | "error" | "looking";
type MessageTone = "info" | "success" | "warning" | "error";
type IconName = "alert" | "camera" | "chart" | "clock" | "close" | "grid" | "image" | "info" | "link" | "play" | "quality" | "refresh" | "settings" | "stop";

interface Pose {
    src: string;
    alt: string;
    animationClass: string;
}

interface CameraConfigResponse {
    resolution: [number, number];
    fps: number;
    target_fps?: number | null;
    jpeg_quality: number;
    mock_camera: boolean;
    max_stream_connections: number;
}

interface LiveMetricsResponse {
    fps_current: number;
    stream_connections: number;
}

interface DiagnosticsResponse {
    camera_ready: boolean;
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

function getElement<T extends HTMLElement = HTMLElement>(id: string): T {
    const element = document.getElementById(id);
    if (!element) throw new Error(`Required dashboard element #${id} was not found`);
    return element as T;
}

const iconMarkup = (name: IconName): string => `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${name}"></use></svg>`;

function escapeHTML(value: unknown): string {
    const entities: Record<string, string> = {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
    };
    return String(value ?? "").replace(/[&<>"']/g, (character) => entities[character]!);
}

function diagnosticMetric(label: string, value: unknown, icon: IconName, valueClass = ""): string {
    return `<div class="metric-row">
        <span class="metric-row__label">${iconMarkup(icon)}<span>${label}</span></span>
        <span class="metric-row__value ${valueClass}">${escapeHTML(value)}</span>
    </div>`;
}

class StreamController {
    private readonly imageElement: HTMLImageElement;
    private readonly loadingElement: HTMLElement;
    private readonly placeholderText: HTMLElement;
    private readonly placeholderDesc: HTMLElement;
    private readonly spinnerElement: HTMLElement;
    private readonly idleElement: HTMLElement;
    private readonly spinnerImage: HTMLImageElement;
    private readonly idleImage: HTMLImageElement;
    private readonly statusIndicator: HTMLElement;
    private readonly statusText: HTMLElement;
    private readonly startButton: HTMLButtonElement;
    private readonly stopButton: HTMLButtonElement;
    isStreaming = false;
    private streamState: StreamState = "ready";
    private statsInterval: number | null = null;
    private cameraConfig: CameraConfigResponse | null = null;
    private cameraConfigExpiresAt = 0;
    private cameraConfigPromise: Promise<CameraConfigResponse> | null = null;
    private streamRetryCount = 0;
    private readonly maxStreamRetries = 3;
    private poseResetTimer: number | null = null;
    private readonly poseByState: Record<PoseState, Pose>;

    constructor() {
        this.imageElement = getElement<HTMLImageElement>("stream-img");
        this.loadingElement = getElement("stream-loading");
        this.placeholderText = getElement("stream-placeholder-text");
        this.placeholderDesc = getElement("stream-placeholder-desc");
        this.spinnerElement = getElement("stream-spinner");
        this.idleElement = getElement("stream-idle");
        this.spinnerImage = getElement<HTMLImageElement>("stream-spinner-image");
        this.idleImage = getElement<HTMLImageElement>("stream-idle-image");
        this.statusIndicator = getElement("status-indicator");
        this.statusText = getElement("status-text");
        this.startButton = getElement<HTMLButtonElement>("start-stream");
        this.stopButton = getElement<HTMLButtonElement>("stop-stream");
        this.poseByState = {
            idle: {
                src: "/static/mio/mio_pose_sleeping.png?v=f7f7212e2ebdf8a6438d93e7d3e4c8a2a7c02a3278d393ba13aad2a7fe040d66",
                alt: "MIO is resting",
                animationClass: "mio-blink",
            },
            loading: {
                src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
                alt: "MIO is curious",
                animationClass: "mio-pulse",
            },
            success: {
                src: "/static/mio/mio_pose_happy.png?v=e2bf5184a25fad3fdd17320965ecc8bd022126f26379ae7df2a857c71ee265be",
                alt: "MIO is happy",
                animationClass: "mio-float",
            },
            warning: {
                src: "/static/mio/mio_pose_concerned.png?v=b28941a29aef2eebe3e74be347dfc9a6e208f61771ec72907e8e7700276f3b5c",
                alt: "MIO is concerned",
                animationClass: "mio-pulse",
            },
            running: {
                src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
                alt: "MIO is attentive",
                animationClass: "mio-pulse",
            },
            error: {
                src: "/static/mio/mio_pose_angry.png?v=45102660b30cef3f208fd7d2663fc9064f421c69dc0a165837c9c52408acc52b",
                alt: "MIO is distressed",
                animationClass: "mio-pulse",
            },
            looking: {
                src: "/static/mio/mio_pose_looking.png?v=004b22d5f48e070762791074142e83cd84314f158005aa5ebf35a5cf64ff9ea6",
                alt: "MIO is scanning",
                animationClass: "mio-float",
            },
        };

        this.updateStatus("ready");
        this.initializeStream();
        this.attachEventListeners();
        this.loadStats();
    }

    getPoseForState(state: string): Pose {
        return this.poseByState[state as PoseState] || this.poseByState.idle;
    }

    applyPoseToImage(imageElement: HTMLImageElement, state: string): void {
        const pose = this.getPoseForState(state);
        imageElement.src = pose.src;
        imageElement.alt = pose.alt;
        imageElement.className = `mio-state-image ${pose.animationClass}`;
    }

    setLoadingPose(state: PoseState = "loading"): void {
        this.applyPoseToImage(this.spinnerImage, state);
        this.spinnerElement.classList.remove("is-hidden");
        this.idleElement.classList.add("is-hidden");
    }

    setIdlePose(state: PoseState = "idle"): void {
        this.applyPoseToImage(this.idleImage, state);
        this.spinnerElement.classList.add("is-hidden");
        this.idleElement.classList.remove("is-hidden");
    }

    clearPoseResetTimer(): void {
        if (this.poseResetTimer !== null) {
            window.clearTimeout(this.poseResetTimer);
            this.poseResetTimer = null;
        }
    }

    schedulePoseResetToIdle(delayMs = 2200): void {
        this.clearPoseResetTimer();
        this.poseResetTimer = window.setTimeout(() => {
            const diagnosticsModal = getElement<HTMLDivElement>("diagnostics-modal");
            const diagnosticsOpen = !diagnosticsModal.hidden;
            if (!this.isStreaming && !diagnosticsOpen) {
                this.setIdlePose("idle");
            }
            this.poseResetTimer = null;
        }, delayMs);
    }

    initializeStream(): void {
        this.imageElement.onload = () => {
            if (!this.imageElement.getAttribute("src")) return;
            this.isStreaming = true;
            this.streamRetryCount = 0;
            this.loadingElement.classList.add("is-hidden");
            this.imageElement.classList.remove("is-hidden");
            this.setIdlePose("success");
            this.idleElement.classList.add("is-hidden");
            this.schedulePoseResetToIdle();
            this.updateStreamControls();
            this.updateStatus("live");
            if (!document.hidden) {
                this.loadStats();
                this.startStatsPolling();
            }
        };

        this.imageElement.onerror = () => {
            // Removing the image source during Stop can also cancel an in-flight request.
            if (!this.imageElement.getAttribute("src")) return;

            this.isStreaming = false;
            this.imageElement.removeAttribute("src");
            this.loadingElement.classList.remove("is-hidden");
            this.imageElement.classList.add("is-hidden");
            this.setIdlePose("warning");

            const nextRetryCount = nextStreamRetryCount(this.streamRetryCount, this.maxStreamRetries);
            if (nextRetryCount !== null) {
                this.streamRetryCount = nextRetryCount;
                this.placeholderText.textContent = "We lost the stream.";
                this.placeholderDesc.textContent = `Reconnecting (${this.streamRetryCount} of ${this.maxStreamRetries})…`;
                this.updateStatus("reconnecting");
                this.showMessage(`MIO is reconnecting (${this.streamRetryCount} of ${this.maxStreamRetries})…`, "warning");
                this.setLoadingPose("running");
                const scheduledRetryCount = nextRetryCount;
                window.setTimeout(() => {
                    if (shouldRunStreamRetry(this.isStreaming, scheduledRetryCount, this.streamRetryCount)) {
                        this.imageElement.src = "/stream.mjpg?t=" + Date.now();
                    }
                }, 2000);
            } else {
                this.placeholderText.textContent = "We couldn't reconnect.";
                this.placeholderDesc.textContent = "Check the camera connection, then try again.";
                this.updateStatus("unavailable");
                this.showMessage("MIO couldn't reconnect. Check the camera connection, then try again.", "error");
            }

            this.updateStreamControls();
            this.stopStatsPolling();
        };
        this.updateStreamControls();
    }

    updateStatus(state: StreamState): void {
        this.streamState = state;
        const presentation = getStreamStatusPresentation(state);
        this.statusText.textContent = presentation.label;
        this.statusIndicator.className = `status-indicator status-indicator--${presentation.tone}`;
    }

    attachEventListeners(): void {
        this.startButton.addEventListener("click", () => this.startStream());
        this.stopButton.addEventListener("click", () => this.stopStream());
    }

    startStream(): void {
        if (this.isStreaming) return;

        this.placeholderText.textContent = "MIO is warming things up…";
        this.placeholderDesc.textContent = "Your camera stream should be ready in a moment.";
        this.loadingElement.classList.remove("is-hidden");
        this.setLoadingPose("loading");
        this.clearPoseResetTimer();
        this.imageElement.classList.add("is-hidden");
        this.updateStatus("connecting");
        this.updateStreamControls();
        this.streamRetryCount = 0;

        // Add cache buster to ensure fresh stream
        this.imageElement.src = "/stream.mjpg?t=" + Date.now();
    }

    stopStream(): void {
        // Call server endpoint to cleanly stop the stream
        requestStopStream()
            .then(() => this.loadStats())
            .catch(err => console.error("Stop stream error:", err));

        // Remove the image source to close the MJPEG request.
        this.imageElement.removeAttribute("src");
        this.isStreaming = false;
        this.streamRetryCount = 0;
        this.imageElement.classList.add("is-hidden");
        this.loadingElement.classList.remove("is-hidden");
        this.setIdlePose("idle");
        this.clearPoseResetTimer();
        this.placeholderText.textContent = "MIO's ready when you are.";
        this.placeholderDesc.textContent = "Start the stream to see what's happening.";
        this.updateStatus("ready");
        this.updateStreamControls();
        this.stopStatsPolling();
    }

    updateStreamControls(): void {
        const canStop = this.streamState === "connecting"
            || this.streamState === "live"
            || this.streamState === "reconnecting";
        this.startButton.disabled = canStop;
        this.stopButton.disabled = !canStop;
    }

    startStatsPolling(): void {
        if (this.statsInterval !== null || !shouldRunDashboardPoller(!document.hidden, this.isStreaming)) return;
        this.statsInterval = window.setInterval(() => {
            this.loadStats();
        }, 2000);
    }

    stopStatsPolling(): void {
        if (this.statsInterval === null) return;
        window.clearInterval(this.statsInterval);
        this.statsInterval = null;
    }

    async loadStats(): Promise<void> {
        try {
            const [config, metricsResponse] = await Promise.all([
                this.loadCameraConfig(),
                requestLiveMetrics(),
            ]);
            if (!metricsResponse.ok) throw new Error("Failed to load live camera metrics");

            const metrics = await metricsResponse.json() as LiveMetricsResponse;
            const aspectRatio = resolutionToAspectRatio(config.resolution);
            if (aspectRatio) {
                getElement("stream-container").style.aspectRatio = aspectRatio;
            }
            getElement("feed-mode-badge").hidden = config.mock_camera !== true;
            // Show measured FPS and color it against the configured target while live.
            const fps = Number(metrics.fps_current);
            const targetFps = config.target_fps ?? Number.NaN;
            const fpsEl = getElement("stat-fps");
            fpsEl.textContent = formatFrameRate(fps);
            fpsEl.classList.remove("metric-card__value--success", "metric-card__value--warning", "metric-card__value--error");
            const fpsTone = getFrameRateTone(fps, targetFps, this.streamState === "live");
            if (fpsTone !== "neutral") fpsEl.classList.add(`metric-card__value--${fpsTone}`);
            getElement("stat-resolution").textContent = formatResolution(config.resolution);
            getElement("stat-quality").textContent = (config.jpeg_quality || "--") + "%";
            getElement("stat-connections").textContent = formatStreamConnections(
                metrics.stream_connections,
                config.max_stream_connections,
            );
        } catch (error) {
            console.error("Error loading stats:", error);
            // Silently fail for stats - don't show error message as this is polling
        }
    }

    private loadCameraConfig(): Promise<CameraConfigResponse> {
        if (this.cameraConfig && Date.now() < this.cameraConfigExpiresAt) {
            return Promise.resolve(this.cameraConfig);
        }
        if (this.cameraConfigPromise) return this.cameraConfigPromise;

        this.cameraConfigPromise = requestCameraConfig()
            .then(async response => {
                if (!response.ok) throw new Error("Failed to load camera configuration");
                const config = await response.json() as CameraConfigResponse;
                this.cameraConfig = config;
                this.cameraConfigExpiresAt = Date.now() + 60_000;
                return config;
            })
            .finally(() => {
                this.cameraConfigPromise = null;
            });
        return this.cameraConfigPromise;
    }

    showMessage(text: string, type: MessageTone = "info"): void {
        const msg = getElement("message");
        msg.textContent = text;
        msg.className = `message notice notice--${type}`;

        if (!this.isStreaming) {
            if (type === "success") {
                this.setIdlePose("success");
                this.schedulePoseResetToIdle(2200);
            } else if (type === "error") {
                this.setIdlePose("error");
                this.schedulePoseResetToIdle(5000);
            } else if (type === "warning") {
                this.setIdlePose("warning");
                this.schedulePoseResetToIdle(3000);
            }
        }

        // Ensure message is visible and animate in
        window.setTimeout(() => {
            msg.classList.add("show");
        }, 10);

        // Auto-dismiss after 4 seconds
        window.setTimeout(() => {
            msg.classList.remove("show");
        }, 4000);
    }
}

// Diagnostics functions
let diagnosticsRefreshInterval: number | null = null;

declare global {
    interface Window {
        streamController?: StreamController;
    }
}

function getStreamController(): StreamController | null {
    return window.streamController || null;
}

function setCompanionPoseFromDiagnostics(data: DiagnosticsResponse): void {
    const controller = getStreamController();
    if (!controller || controller.isStreaming) return;

    if (!data.camera_ready && (data.capture_failures_consecutive || 0) > 3) {
        controller.setIdlePose("error");
        return;
    }

    if (!data.camera_ready || (data.error_rate_percent || 0) > 20 || (data.capture_failures_consecutive || 0) > 0) {
        controller.setIdlePose("warning");
        return;
    }

    if ((data.stream_connections || 0) > 0) {
        controller.setIdlePose("running");
        return;
    }

    if ((data.error_rate_percent || 0) > 5) {
        controller.setIdlePose("warning");
        return;
    }

    controller.setIdlePose("success");
}

let diagnosticsReturnFocus: ReturnFocusTarget | null = null;

function openDiagnosticsModal(): void {
    const modal = getElement<HTMLDivElement>("diagnostics-modal");
    if (!modal.hidden) return;

    diagnosticsReturnFocus = rememberReturnFocus(document);
    modal.hidden = false;
    modal.classList.add("is-open");
    document.body.classList.add("modal-open");
    getElement<HTMLButtonElement>("diagnostics-close").focus();

    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
        controller.setLoadingPose("looking");
    }
    startDiagnosticsPolling();
}

function closeDiagnosticsModal(): void {
    const modal = getElement<HTMLDivElement>("diagnostics-modal");
    if (modal.hidden) return;

    modal.classList.remove("is-open");
    modal.hidden = true;
    document.body.classList.remove("modal-open");
    stopDiagnosticsPolling();
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
        controller.setIdlePose("idle");
    }
    restoreReturnFocus(diagnosticsReturnFocus);
    diagnosticsReturnFocus = null;
}

function startDiagnosticsPolling(): void {
    stopDiagnosticsPolling();
    const modal = getElement<HTMLDivElement>("diagnostics-modal");
    if (!shouldRunDashboardPoller(!document.hidden, !modal.hidden)) return;
    fetchDiagnostics();
    diagnosticsRefreshInterval = window.setInterval(fetchDiagnostics, 3000);
}

function stopDiagnosticsPolling(): void {
    if (diagnosticsRefreshInterval === null) return;
    window.clearInterval(diagnosticsRefreshInterval);
    diagnosticsRefreshInterval = null;
}

function handleDashboardVisibilityChange(): void {
    const controller = getStreamController();
    if (document.hidden) {
        controller?.stopStatsPolling();
        stopDiagnosticsPolling();
        return;
    }

    if (controller?.isStreaming) {
        controller.loadStats();
        controller.startStatsPolling();
    }
    if (!getElement<HTMLDivElement>("diagnostics-modal").hidden) {
        startDiagnosticsPolling();
    }
}

function fetchDiagnostics(): void {
    requestDiagnostics()
        .then((response) => {
            if (!response.ok) throw new Error("Failed to load diagnostics");
            return response.json() as Promise<DiagnosticsResponse>;
        })
        .then((data) => {
            setCompanionPoseFromDiagnostics(data);
            const content = getElement<HTMLDivElement>("diagnostics-content");
            const errorRate = Number(data.error_rate_percent || 0);
            const consecutiveFailures = Number(data.capture_failures_consecutive || 0);
            const healthTone = getHealthTone(data.health_status, data.camera_ready, errorRate, consecutiveFailures);
            const cameraTone = data.camera_ready ? "success" : "error";
            const cameraStatus = data.camera_ready ? "Ready" : "Unavailable";
            const cameraCard = `<div class="metric-card metric-card--status metric-card--${cameraTone}">
                <div class="metric-card__label">Camera</div>
                <div class="metric-card__value">${cameraStatus}</div>
            </div>`;
            const healthCard = `<div class="metric-card metric-card--status metric-card--${healthTone}">
                <div class="metric-card__label">System health</div>
                <div class="metric-card__value">${escapeHTML(data.health_status || "Unknown")}</div>
            </div>`;
            const totalFailures = Number(data.capture_failures_total || 0);
            const noticeTone = healthTone === "neutral" ? "info" : healthTone;

            content.innerHTML = `
                <div class="diagnostic-summary-grid">
                    ${cameraCard}
                    ${healthCard}
                </div>

                <div class="diagnostic-summary-grid">
                    <div class="metric-card">
                        <div class="metric-card__label">${iconMarkup("clock")} Uptime</div>
                        <div class="metric-card__value">${formatUptime(Number(data.uptime_seconds || 0))}</div>
                    </div>
                    <div class="metric-card">
                        <div class="metric-card__label">${iconMarkup("alert")} Error rate</div>
                        <div class="metric-card__value metric-card__value--${healthTone}">${errorRate.toFixed(1)}%</div>
                    </div>
                </div>

                <section class="diagnostics-details" aria-labelledby="diagnostics-metrics-title">
                    <h3 class="diagnostics-subtitle" id="diagnostics-metrics-title">Detailed metrics</h3>
                    <div class="diagnostic-metric-list">
                        ${diagnosticMetric("Frame rate", formatFrameRate(Number(data.fps_current ?? Number.NaN)), "camera")}
                        ${diagnosticMetric("Resolution", formatResolution(data.resolution), "grid")}
                        ${diagnosticMetric("JPEG quality", `${Number(data.jpeg_quality || 0)}%`, "quality")}
                        ${diagnosticMetric("Frames captured", Number(data.frames_captured || 0).toLocaleString(), "image")}
                        ${diagnosticMetric("Last frame age", formatFrameAge(data.last_frame_age_seconds), "clock")}
                        ${diagnosticMetric("Active streams", formatStreamConnections(Number(data.stream_connections || 0), Number(data.max_stream_connections || 0)), "link")}
                        ${diagnosticMetric("Recent failures", consecutiveFailures, "alert", consecutiveFailures > 0 ? "metric-card__value--error" : "")}
                        ${diagnosticMetric("Total failures", totalFailures, "chart")}
                    </div>
                </section>

                <div class="notice notice--${noticeTone} diagnostic-notice">
                    <strong>${iconMarkup(noticeTone === "error" ? "alert" : "info")} System status</strong>
                    <p>${escapeHTML(data.message || "No status details right now.")}</p>
                </div>

                <div class="refresh-indicator">${iconMarkup("refresh")}<span>Refreshes every 3 seconds</span></div>
            `;
        })
        .catch((error) => {
            console.error("Diagnostics request failed:", error);
            const controller = getStreamController();
            if (controller && !controller.isStreaming) {
                controller.setIdlePose("warning");
            }
            const content = getElement<HTMLDivElement>("diagnostics-content");
            content.innerHTML = `<div class="notice notice--error message show" role="alert">MIO couldn't load diagnostics just now. Please try again in a moment.</div>`;
        });
}

function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;

    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    if (mins > 0) return `${mins}m ${secs}s`;
    return `${secs}s`;
}

// Keyboard accessibility for modal
document.addEventListener("keydown", (event: KeyboardEvent) => {
    const modal = getElement<HTMLDivElement>("diagnostics-modal");
    handleKeydown(event, modal, closeDiagnosticsModal, document);
});

// Close modal when clicking outside
getElement<HTMLDivElement>("diagnostics-modal").addEventListener("click", (event: MouseEvent) => {
    if (event.target === event.currentTarget) closeDiagnosticsModal();
});

// Initialize when DOM is ready
document.addEventListener("DOMContentLoaded", () => {
    window.streamController = new StreamController();
    getElement<HTMLButtonElement>("diagnostics-btn").addEventListener("click", openDiagnosticsModal);
    getElement<HTMLButtonElement>("diagnostics-close").addEventListener("click", closeDiagnosticsModal);
    document.addEventListener("visibilitychange", handleDashboardVisibilityChange);
});
