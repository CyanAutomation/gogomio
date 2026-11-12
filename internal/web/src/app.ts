import { resolutionToAspectRatio } from "./aspect-ratio";
import { requestCameraConfig, requestCameraReadiness, requestDiagnostics, requestLiveMetrics, requestStopStream } from "./api";
import {
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getCameraReadinessState,
    getCameraReadinessPresentation,
    getFrameRateTone,
    getHealthTone,
    getStreamMascotPresentation,
    getStreamControlPresentation,
    getStreamStatusPresentation,
    type CameraReadinessState,
    type StreamState,
} from "./dashboard-presentation";
import { closeNativeDialog, openNativeDialog } from "./diagnostics-dialog";
import { nextStreamRetryCount, shouldRunStreamRetry } from "./stream-retry";
import { startDashboardPoller } from "./polling";
import { SnackbarController, type SnackbarTone } from "./snackbar";
import {
    escapeHTML,
    renderMetricCard,
    renderMetricRow,
    renderStatusBadge,
    updateMascotImage,
    updateStatusBadge,
    type MetricRowIcon,
    type MetricRowTone,
} from "./ui-components";

type PoseState = "idle" | "loading" | "success" | "warning" | "running" | "error" | "looking";
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

interface CameraReadinessResponse {
    status: string;
}

function getElement<T extends HTMLElement = HTMLElement>(id: string): T {
    const element = document.getElementById(id);
    if (!element) throw new Error(`Required dashboard element #${id} was not found`);
    return element as T;
}

const iconMarkup = (name: IconName): string => `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${name}"></use></svg>`;

function diagnosticMetric(label: string, value: unknown, icon: MetricRowIcon, tone: MetricRowTone = "neutral"): string {
    return renderMetricRow(label, value, icon, tone);
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
    private readonly streamStatusBadge: HTMLElement;
    private readonly statusText: HTMLElement;
    private readonly streamStatusMascot: HTMLImageElement;
    private readonly cameraStatusBadge: HTMLElement;
    private readonly cameraStatusText: HTMLElement;
    private readonly cameraStatusGuidance: HTMLElement;
    private readonly startButton: HTMLButtonElement;
    private readonly stopButton: HTMLButtonElement;
    private readonly snackbar: SnackbarController;
    isStreaming = false;
    private streamState: StreamState = "ready";
    private statsInterval: number | null = null;
    private cameraReadinessInterval: number | null = null;
    private cameraReadinessPromise: Promise<void> | null = null;
    private cameraReadiness: CameraReadinessState = "checking";
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
        this.streamStatusBadge = getElement("stream-status-badge");
        this.statusText = getElement("status-text");
        this.streamStatusMascot = getElement<HTMLImageElement>("stream-status-mascot");
        this.cameraStatusBadge = getElement("camera-status-badge");
        this.cameraStatusText = getElement("camera-status-text");
        this.cameraStatusGuidance = getElement("camera-status-guidance");
        this.startButton = getElement<HTMLButtonElement>("start-stream");
        this.stopButton = getElement<HTMLButtonElement>("stop-stream");
        const snackbarElement = getElement("snackbar");
        const snackbarText = getElement("snackbar-text");
        this.snackbar = new SnackbarController({
            show: (text: string, tone: SnackbarTone) => {
                snackbarText.textContent = text;
                snackbarElement.className = `snackbar notice notice--${tone} is-visible`;
            },
            hide: () => snackbarElement.classList.remove("is-visible"),
        });
        snackbarElement.addEventListener("mouseenter", () => this.snackbar.pause());
        snackbarElement.addEventListener("mouseleave", () => {
            if (!snackbarElement.contains(document.activeElement)) this.snackbar.resume();
        });
        snackbarElement.addEventListener("focusin", () => this.snackbar.pause());
        snackbarElement.addEventListener("focusout", event => {
            if (!snackbarElement.contains(event.relatedTarget as Node | null) && !snackbarElement.matches(":hover")) {
                this.snackbar.resume();
            }
        });
        getElement<HTMLButtonElement>("snackbar-dismiss").addEventListener("click", () => this.snackbar.dismiss());
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
                src: "/static/mio/mio_pose_worried.png?v=2c136a60a29ff48447aa0bd6484050e7af02efa87cbcab7279db3455215f92e4",
                alt: "MIO is worried",
                animationClass: "mio-pulse",
            },
            looking: {
                src: "/static/mio/mio_pose_looking.png?v=004b22d5f48e070762791074142e83cd84314f158005aa5ebf35a5cf64ff9ea6",
                alt: "MIO is scanning",
                animationClass: "mio-float",
            },
        };

        this.updateStatus("ready");
        this.updateCameraReadiness("checking");
        this.initializeStream();
        this.attachEventListeners();
        this.loadCameraReadiness();
        this.startCameraReadinessPolling();
        this.loadStats();
    }

    getPoseForState(state: string): Pose {
        return this.poseByState[state as PoseState] || this.poseByState.idle;
    }

    applyPoseToImage(imageElement: HTMLImageElement, state: string): void {
        const pose = this.getPoseForState(state);
        imageElement.src = pose.src;
        imageElement.alt = pose.alt;
        imageElement.className = `mio-avatar mio-avatar--state ${pose.animationClass}`;
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
            const diagnosticsModal = getElement<HTMLDialogElement>("diagnostics-modal");
            const diagnosticsOpen = diagnosticsModal.open;
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
            this.updateStatus("live");
            this.updateStreamControls();
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
                this.placeholderText.textContent = "MIO lost the stream for a moment.";
                this.placeholderDesc.textContent = `MIO is reconnecting (${this.streamRetryCount} of ${this.maxStreamRetries})…`;
                this.updateStatus("reconnecting");
                this.setLoadingPose("warning");
                const scheduledRetryCount = nextRetryCount;
                window.setTimeout(() => {
                    if (shouldRunStreamRetry(this.isStreaming, scheduledRetryCount, this.streamRetryCount)) {
                        this.imageElement.src = "/stream.mjpg?t=" + Date.now();
                    }
                }, 2000);
            } else {
                this.placeholderText.textContent = "MIO couldn't reconnect.";
                this.placeholderDesc.textContent = "Check the camera connection, then try again.";
                this.setLoadingPose("error");
                this.updateStatus("unavailable");
            }

            this.updateStreamControls();
            this.stopStatsPolling();
        };
        this.updateStreamControls();
    }

    updateStatus(state: StreamState): void {
        this.streamState = state;
        const presentation = getStreamStatusPresentation(state);
        updateStatusBadge(
            { root: this.streamStatusBadge, label: this.statusText },
            presentation.label,
            presentation.tone,
        );
        updateMascotImage(this.streamStatusMascot, getStreamMascotPresentation(state), "status", true);
    }

    updateCameraReadiness(state: CameraReadinessState): void {
        if (this.cameraReadiness === state) return;
        this.cameraReadiness = state;
        const presentation = getCameraReadinessPresentation(state);
        updateStatusBadge(
            { root: this.cameraStatusBadge, label: this.cameraStatusText },
            presentation.label,
            presentation.tone,
        );
        this.cameraStatusGuidance.textContent = presentation.guidance;
        this.cameraStatusGuidance.hidden = presentation.guidance.length === 0;
    }

    loadCameraReadiness(): Promise<void> {
        if (this.cameraReadinessPromise) return this.cameraReadinessPromise;
        this.cameraReadinessPromise = requestCameraReadiness()
            .then(async response => {
                const data = response.ok
                    ? await response.json() as CameraReadinessResponse
                    : undefined;
                this.updateCameraReadiness(getCameraReadinessState(response.ok, data?.status));
            })
            .catch(() => this.updateCameraReadiness("unknown"))
            .finally(() => {
                this.cameraReadinessPromise = null;
            });
        return this.cameraReadinessPromise;
    }

    startCameraReadinessPolling(): void {
        if (this.cameraReadinessInterval !== null) return;
        this.cameraReadinessInterval = startDashboardPoller(
            !document.hidden,
            true,
            5000,
            () => { void this.loadCameraReadiness(); },
        );
    }

    stopCameraReadinessPolling(): void {
        if (this.cameraReadinessInterval === null) return;
        window.clearInterval(this.cameraReadinessInterval);
        this.cameraReadinessInterval = null;
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
        this.snackbar.show("Stream stopped.", "success");
    }

    updateStreamControls(): void {
        const presentation = getStreamControlPresentation(this.streamState);
        const activeControl = document.activeElement === this.startButton
            ? "start"
            : document.activeElement === this.stopButton ? "stop" : null;
        this.startButton.hidden = presentation.startHidden;
        this.startButton.disabled = presentation.startHidden;
        this.startButton.querySelector("span")!.textContent = presentation.startLabel;
        this.stopButton.hidden = presentation.stopHidden;
        this.stopButton.disabled = presentation.stopHidden;
        this.stopButton.querySelector("span")!.textContent = presentation.stopLabel;
        this.stopButton.classList.toggle("button--primary", presentation.stopPrimary);
        this.stopButton.classList.toggle("button--secondary", !presentation.stopPrimary);

        if (activeControl === "start" && presentation.startHidden) this.stopButton.focus();
        if (activeControl === "stop" && presentation.stopHidden) this.startButton.focus();
    }

    startStatsPolling(): void {
        if (this.statsInterval !== null) return;
        this.statsInterval = startDashboardPoller(!document.hidden, this.isStreaming, 2000, () => {
            this.loadStats();
        });
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
            fpsEl.textContent = this.streamState === "live" ? formatFrameRate(fps) : "--";
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

function openDiagnosticsModal(): void {
    const modal = getElement<HTMLDialogElement>("diagnostics-modal");
    const closeButton = getElement<HTMLButtonElement>("diagnostics-close");
    if (!openNativeDialog(modal, closeButton)) return;

    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
        controller.setLoadingPose("looking");
    }
    startDiagnosticsPolling();
}

function closeDiagnosticsModal(): void {
    const modal = getElement<HTMLDialogElement>("diagnostics-modal");
    closeNativeDialog(modal);
}

function handleDiagnosticsDialogClose(): void {
    stopDiagnosticsPolling();
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
        controller.setIdlePose("idle");
    }
}

function startDiagnosticsPolling(): void {
    stopDiagnosticsPolling();
    const modal = getElement<HTMLDialogElement>("diagnostics-modal");
    const interval = startDashboardPoller(!document.hidden, modal.open, 3000, fetchDiagnostics);
    if (interval === null) return;
    fetchDiagnostics();
    diagnosticsRefreshInterval = interval;
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
        controller?.stopCameraReadinessPolling();
        stopDiagnosticsPolling();
        return;
    }

    controller?.loadCameraReadiness();
    controller?.startCameraReadinessPolling();
    if (controller?.isStreaming) {
        controller.loadStats();
        controller.startStatsPolling();
    }
    if (getElement<HTMLDialogElement>("diagnostics-modal").open) {
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
            getStreamController()?.updateCameraReadiness(data.camera_ready ? "ready" : "unavailable");
            const content = getElement<HTMLDivElement>("diagnostics-content");
            const errorRate = Number(data.error_rate_percent || 0);
            const consecutiveFailures = Number(data.capture_failures_consecutive || 0);
            const healthTone = getHealthTone(data.health_status, data.camera_ready, errorRate, consecutiveFailures);
            const cameraTone = data.camera_ready ? "success" : "error";
            const cameraStatus = data.camera_ready ? "Ready" : "Unavailable";
            const cameraCard = `<div class="metric-card metric-card--status">
                <div class="metric-card__label">Camera</div>
                ${renderStatusBadge(cameraStatus, cameraTone)}
            </div>`;
            const healthCard = `<div class="metric-card metric-card--status">
                <div class="metric-card__label">System health</div>
                ${renderStatusBadge(data.health_status || "Unknown", healthTone)}
            </div>`;
            const totalFailures = Number(data.capture_failures_total || 0);
            const noticeTone = healthTone === "neutral" ? "info" : healthTone;
            const existingDetails = content.querySelector<HTMLDetailsElement>(".diagnostics-details");
            const detailsWereOpen = existingDetails?.open ?? false;
            const detailsHadFocus = existingDetails?.contains(document.activeElement) ?? false;

            content.innerHTML = `
                <div class="diagnostic-summary-grid">
                    ${cameraCard}
                    ${healthCard}
                </div>

                <div class="notice notice--${noticeTone} diagnostic-notice">
                    <strong>${iconMarkup(noticeTone === "error" ? "alert" : "info")} System status</strong>
                    <p>${escapeHTML(data.message || "No status details right now.")}</p>
                </div>

                <details class="diagnostics-details">
                    <summary class="disclosure-summary">Detailed metrics</summary>
                    <div class="diagnostics-secondary-grid">
                        ${renderMetricCard("Uptime", formatUptime(Number(data.uptime_seconds || 0)))}
                        ${renderMetricCard("Error rate", `${errorRate.toFixed(1)}%`, healthTone)}
                    </div>

                    <div class="diagnostic-metric-list">
                        ${diagnosticMetric("Frame rate", formatFrameRate(Number(data.fps_current ?? Number.NaN)), "camera")}
                        ${diagnosticMetric("Resolution", formatResolution(data.resolution), "grid")}
                        ${diagnosticMetric("JPEG quality", `${Number(data.jpeg_quality || 0)}%`, "quality")}
                        ${diagnosticMetric("Frames captured", Number(data.frames_captured || 0).toLocaleString(), "image")}
                        ${diagnosticMetric("Last frame age", formatFrameAge(data.last_frame_age_seconds), "clock")}
                        ${diagnosticMetric("Active streams", formatStreamConnections(Number(data.stream_connections || 0), Number(data.max_stream_connections || 0)), "link")}
                        ${diagnosticMetric("Recent failures", consecutiveFailures, "alert", consecutiveFailures > 0 ? "error" : "neutral")}
                        ${diagnosticMetric("Total failures", totalFailures, "chart")}
                    </div>
                </details>

                <div class="refresh-indicator">${iconMarkup("refresh")}<span>Refreshes every 3 seconds</span></div>
            `;

            const updatedDetails = content.querySelector<HTMLDetailsElement>(".diagnostics-details");
            if (updatedDetails) {
                updatedDetails.open = detailsWereOpen;
                if (detailsHadFocus) updatedDetails.querySelector("summary")?.focus();
            }
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

// Initialize when DOM is ready
document.addEventListener("DOMContentLoaded", () => {
    window.streamController = new StreamController();
    getElement<HTMLButtonElement>("diagnostics-btn").addEventListener("click", openDiagnosticsModal);
    getElement<HTMLButtonElement>("diagnostics-close").addEventListener("click", closeDiagnosticsModal);
    getElement<HTMLDialogElement>("diagnostics-modal").addEventListener("close", handleDiagnosticsDialogClose);
    document.addEventListener("visibilitychange", handleDashboardVisibilityChange);
});
