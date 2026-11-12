import { resolutionToAspectRatio } from "./aspect-ratio";
import { requestCameraConfig, requestCameraReadiness, requestDiagnostics, requestLiveMetrics, requestStopStream } from "./api";
import {
    ConsecutivePollHysteresis,
    MIO_SLEEP_AFTER_MS,
    formatFrameAge,
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getCameraReadinessState,
    getCameraReadinessPresentation,
    getFrameRateTone,
    getCompanionPoseForFrameRateTone,
    getHealthTone,
    getMioPosePresentation,
    getStreamControlPresentation,
    getStreamDescription,
    getStreamStatusPresentation,
    isCapacitySignal,
    isMioSleepDue,
    type CameraReadinessState,
    type MioPose,
    type StreamState,
} from "./dashboard-presentation";
import { closeNativeDialog, openNativeDialog } from "./diagnostics-dialog";
import { nextStreamRetryCount, shouldRunStreamRetry } from "./stream-retry";
import { startDashboardPoller } from "./polling";
import { SnackbarController, type SnackbarTone } from "./snackbar";
import {
    renderIcon,
    renderMetricCard,
    renderMetricRow,
    renderNotice,
    renderSectionHeading,
    renderStatusMetricCard,
    updateMascotImage,
    updateStatusBadge,
    type MetricRowIcon,
    type MetricRowTone,
} from "./ui-components";

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

function diagnosticMetric(label: string, value: unknown, icon: MetricRowIcon, tone: MetricRowTone = "neutral"): string {
    return renderMetricRow(label, value, icon, tone);
}

class StreamController {
    private readonly imageElement: HTMLImageElement;
    private readonly loadingElement: HTMLElement;
    private readonly placeholderText: HTMLElement;
    private readonly placeholderDesc: HTMLElement;
    private readonly placeholderImage: HTMLImageElement;
    private readonly sleepDecoration: HTMLElement;
    private readonly headerMascot: HTMLImageElement;
    private readonly footerMascot: HTMLImageElement;
    private readonly viewerMascot: HTMLImageElement;
    private readonly liveSparkle: HTMLElement;
    private readonly pageDescription: HTMLElement;
    private readonly fpsValue: HTMLElement;
    private readonly fpsIdle: HTMLElement;
    private readonly fpsMascot: HTMLImageElement;
    private readonly streamStatusBadge: HTMLElement;
    private readonly statusText: HTMLElement;
    private readonly cameraStatusBadge: HTMLElement;
    private readonly cameraStatusText: HTMLElement;
    private readonly cameraStatusGuidance: HTMLElement;
    private readonly startButton: HTMLButtonElement;
    private readonly stopButton: HTMLButtonElement;
    private readonly snackbar: SnackbarController;
    private streaming = false;
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
    private sleepTimer: number | null = null;
    private idleSince = Date.now();
    private placeholderPose: MioPose = "idle";
    private readonly viewerPoseHysteresis = new ConsecutivePollHysteresis<MioPose>("happy", 2);
    private readonly preloadedSprites: HTMLImageElement[] = [];

    get isStreaming(): boolean {
        return this.streaming;
    }

    constructor() {
        this.imageElement = getElement<HTMLImageElement>("stream-img");
        this.loadingElement = getElement("stream-loading");
        this.placeholderText = getElement("stream-placeholder-text");
        this.placeholderDesc = getElement("stream-placeholder-desc");
        this.placeholderImage = getElement<HTMLImageElement>("stream-placeholder-mio");
        this.sleepDecoration = getElement("stream-sleep-z");
        this.headerMascot = getElement<HTMLImageElement>("header-mio");
        this.footerMascot = getElement<HTMLImageElement>("footer-mio");
        this.viewerMascot = getElement<HTMLImageElement>("stream-companion");
        this.liveSparkle = getElement("stream-live-sparkle");
        this.pageDescription = getElement("page-description");
        this.fpsValue = getElement("stat-fps");
        this.fpsIdle = getElement("stat-fps-idle");
        this.fpsMascot = getElement<HTMLImageElement>("fps-idle-mio");
        this.streamStatusBadge = getElement("stream-status-badge");
        this.statusText = getElement("status-text");
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

        const readyPose = getMioPosePresentation("idle");
        updateMascotImage(this.headerMascot, readyPose, "brand");
        updateMascotImage(this.footerMascot, readyPose, "footer", true);
        updateMascotImage(this.fpsMascot, getMioPosePresentation("sleeping"), "fps", true);
        this.setPlaceholderPose("idle");
        this.updateStatus("ready");
        this.updateFPSVisibility();
        this.updateCameraReadiness("checking");
        this.initializeStream();
        this.attachEventListeners();
        this.loadCameraReadiness();
        this.startCameraReadinessPolling();
        this.loadStats();
        this.scheduleInactivitySleep();
        this.preloadSpritesAfterFirstPaint();
    }

    private preloadSpritesAfterFirstPaint(): void {
        window.requestAnimationFrame(() => {
            window.setTimeout(() => {
                for (const pose of ["idle", "sleeping", "curious", "concerned", "happy", "worried", "angry", "looking"] as const) {
                    const image = new Image();
                    image.decoding = "async";
                    image.src = getMioPosePresentation(pose).src;
                    this.preloadedSprites.push(image);
                }
            }, 200);
        });
    }

    setPlaceholderPose(pose: MioPose): void {
        this.placeholderPose = pose;
        updateMascotImage(this.placeholderImage, getMioPosePresentation(pose), "placeholder", true);
        this.sleepDecoration.hidden = pose !== "sleeping";
        this.loadingElement.classList.toggle("is-sleeping", pose === "sleeping");
    }

    private setViewerPose(pose: MioPose): void {
        updateMascotImage(this.viewerMascot, getMioPosePresentation(pose), "viewer", true);
    }

    private setViewerPoseFromTone(tone: ReturnType<typeof getFrameRateTone>): void {
        const nextPose = getCompanionPoseForFrameRateTone(tone);
        this.setViewerPose(this.viewerPoseHysteresis.update(nextPose));
    }

    private clearInactivityTimer(): void {
        if (this.sleepTimer === null) return;
        window.clearTimeout(this.sleepTimer);
        this.sleepTimer = null;
    }

    private scheduleInactivitySleep(): void {
        this.clearInactivityTimer();
        if (this.streaming || this.streamState !== "ready") return;
        this.idleSince = Date.now();
        this.sleepTimer = window.setTimeout(() => {
            this.sleepTimer = null;
            if (this.streamState !== "ready"
                || this.streaming
                || getElement<HTMLDialogElement>("diagnostics-modal").open
                || !isMioSleepDue(this.idleSince, Date.now())) return;
            this.setPlaceholderPose("sleeping");
            this.placeholderText.textContent = "MIO's taking a little rest.";
            this.placeholderDesc.textContent = "Start the stream whenever you're ready.";
        }, MIO_SLEEP_AFTER_MS);
    }

    private handleUserActivity(): void {
        if (this.streamState !== "ready") return;
        if (this.placeholderPose === "sleeping") {
            this.setPlaceholderPose("idle");
            this.placeholderText.textContent = "MIO's ready when you are.";
            this.placeholderDesc.textContent = "Start the stream to see what's happening.";
        }
        this.scheduleInactivitySleep();
    }

    private initializeStream(): void {
        this.imageElement.onload = () => {
            if (!this.imageElement.getAttribute("src")) return;
            this.streaming = true;
            this.streamRetryCount = 0;
            this.clearInactivityTimer();
            this.loadingElement.classList.add("is-hidden");
            this.imageElement.classList.remove("is-hidden");
            this.setPlaceholderPose("happy");
            this.viewerPoseHysteresis.reset("happy");
            this.setViewerPose("happy");
            this.viewerMascot.hidden = false;
            getElement("stream-container").classList.add("is-stream-live");
            this.playLiveSparkleOnce();
            this.updateStatus("live");
            this.updateStreamControls();
            if (!document.hidden) {
                this.loadStats();
                this.startStatsPolling();
            }
        };

        this.imageElement.onerror = () => {
            const source = this.imageElement.getAttribute("src");
            // Removing the image source during Stop can also cancel an in-flight request.
            if (!source) return;
            const responseStatus = getImageResponseStatus(source);

            this.streaming = false;
            this.imageElement.removeAttribute("src");
            this.loadingElement.classList.remove("is-hidden");
            this.imageElement.classList.add("is-hidden");
            this.viewerMascot.hidden = true;
            getElement("stream-container").classList.remove("is-stream-live");
            this.liveSparkle.classList.remove("is-sparkling");

            if (isCapacitySignal(0, 0, false, responseStatus)) {
                this.showCapacityState();
                this.stopStatsPolling();
                return;
            }

            const nextRetryCount = nextStreamRetryCount(this.streamRetryCount, this.maxStreamRetries);
            if (nextRetryCount !== null) {
                this.streamRetryCount = nextRetryCount;
                this.placeholderText.textContent = "MIO lost the stream for a moment.";
                this.placeholderDesc.textContent = getStreamDescription("reconnecting", this.streamRetryCount, this.maxStreamRetries);
                this.setPlaceholderPose("concerned");
                this.updateStatus("reconnecting");
                const scheduledRetryCount = nextRetryCount;
                window.setTimeout(() => {
                    if (shouldRunStreamRetry(this.streaming, scheduledRetryCount, this.streamRetryCount)) {
                        this.imageElement.src = "/stream.mjpg?t=" + Date.now();
                    }
                }, 2000);
            } else {
                this.placeholderText.textContent = "MIO couldn't reconnect.";
                this.placeholderDesc.textContent = "Check the camera connection, then try again.";
                this.setPlaceholderPose("worried");
                this.updateStatus("unavailable");
            }

            this.updateStreamControls();
            this.updateFPSVisibility();
            this.stopStatsPolling();
        };
        this.updateStreamControls();
    }

    private playLiveSparkleOnce(): void {
        if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
        this.liveSparkle.classList.remove("is-sparkling");
        void this.liveSparkle.offsetWidth;
        this.liveSparkle.classList.add("is-sparkling");
    }

    showCapacityState(): void {
        this.streaming = false;
        this.streamRetryCount = 0;
        this.placeholderText.textContent = "MIO's at capacity right now.";
        this.placeholderDesc.textContent = "Try again when another stream is free.";
        this.setPlaceholderPose("angry");
        this.updateStatus("capacity");
        this.updateStreamControls();
        this.updateFPSVisibility();
    }

    updateStatus(state: StreamState): void {
        this.streamState = state;
        const presentation = getStreamStatusPresentation(state);
        updateStatusBadge(
            { root: this.streamStatusBadge, label: this.statusText },
            presentation.label,
            presentation.tone,
        );
        this.pageDescription.textContent = getStreamDescription(state, Math.max(1, this.streamRetryCount), this.maxStreamRetries);
        this.updateFPSVisibility();
        if (state !== "ready") this.clearInactivityTimer();
    }

    private restorePoseAfterDiagnostics(): void {
        if (this.streaming) return;
        if (this.streamState === "capacity") this.setPlaceholderPose("angry");
        else if (this.streamState === "unavailable") this.setPlaceholderPose("worried");
        else if (this.streamState === "reconnecting") this.setPlaceholderPose("concerned");
        else if (this.streamState === "connecting") this.setPlaceholderPose("curious");
        else if (this.placeholderPose !== "sleeping") this.setPlaceholderPose("idle");
        this.scheduleInactivitySleep();
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

    beginDiagnostics(): void {
        if (!this.streaming) this.setPlaceholderPose("looking");
    }

    endDiagnostics(): void {
        this.restorePoseAfterDiagnostics();
    }

    attachEventListeners(): void {
        this.startButton.addEventListener("click", () => this.startStream());
        this.stopButton.addEventListener("click", () => this.stopStream());
        document.addEventListener("pointerdown", () => this.handleUserActivity());
        document.addEventListener("keydown", () => this.handleUserActivity());
    }

    startStream(): void {
        if (this.streaming) return;

        this.clearInactivityTimer();
        this.placeholderText.textContent = "MIO is warming things up…";
        this.placeholderDesc.textContent = "Your camera stream should be ready in a moment.";
        this.loadingElement.classList.remove("is-hidden");
        this.setPlaceholderPose("curious");
        this.imageElement.classList.add("is-hidden");
        this.updateStatus("connecting");
        this.updateStreamControls();
        this.streamRetryCount = 0;

        // Add a cache buster to open a fresh MJPEG request.
        this.imageElement.src = "/stream.mjpg?t=" + Date.now();
    }

    stopStream(): void {
        requestStopStream()
            .then(() => this.loadStats())
            .catch(err => console.error("Stop stream error:", err));

        this.imageElement.removeAttribute("src");
        this.streaming = false;
        this.streamRetryCount = 0;
        this.imageElement.classList.add("is-hidden");
        this.loadingElement.classList.remove("is-hidden");
        this.viewerMascot.hidden = true;
        getElement("stream-container").classList.remove("is-stream-live");
        this.liveSparkle.classList.remove("is-sparkling");
        this.setPlaceholderPose("idle");
        this.placeholderText.textContent = "MIO's ready when you are.";
        this.placeholderDesc.textContent = "Start the stream to see what's happening.";
        this.updateStatus("ready");
        this.updateStreamControls();
        this.stopStatsPolling();
        this.scheduleInactivitySleep();
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
        this.statsInterval = startDashboardPoller(!document.hidden, this.streaming, 2000, () => {
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
            if (aspectRatio) getElement("stream-container").style.aspectRatio = aspectRatio;
            getElement("feed-mode-badge").hidden = config.mock_camera !== true;

            const fps = Number(metrics.fps_current);
            const targetFps = config.target_fps ?? Number.NaN;
            this.fpsValue.textContent = this.streaming ? formatFrameRate(fps) : "";
            this.fpsValue.classList.remove("metric-card__value--success", "metric-card__value--warning", "metric-card__value--error");
            const fpsTone = getFrameRateTone(fps, targetFps, this.streaming);
            if (fpsTone !== "neutral") this.fpsValue.classList.add(`metric-card__value--${fpsTone}`);
            if (this.streaming) this.setViewerPoseFromTone(fpsTone);
            this.updateFPSVisibility();

            getElement("stat-resolution").textContent = formatResolution(config.resolution);
            getElement("stat-quality").textContent = (config.jpeg_quality || "--") + "%";
            getElement("stat-connections").textContent = formatStreamConnections(
                metrics.stream_connections,
                config.max_stream_connections,
            );

            if (isCapacitySignal(metrics.stream_connections, config.max_stream_connections, this.streaming)
                && this.streamState === "ready") {
                this.showCapacityState();
            } else if (this.streamState === "capacity"
                && !isCapacitySignal(metrics.stream_connections, config.max_stream_connections, this.streaming)) {
                this.updateStatus("ready");
                this.setPlaceholderPose("idle");
                this.placeholderText.textContent = "MIO's ready when you are.";
                this.placeholderDesc.textContent = "Start the stream to see what's happening.";
                this.updateStreamControls();
                this.scheduleInactivitySleep();
            }
        } catch (error) {
            console.error("Error loading stats:", error);
        }
    }

    private updateFPSVisibility(): void {
        const live = this.streamState === "live";
        this.fpsValue.hidden = !live;
        this.fpsIdle.hidden = live;
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

function getImageResponseStatus(source: string): number {
    const absoluteSource = new URL(source, window.location.href).href;
    const entries = performance.getEntriesByName(absoluteSource, "resource") as Array<PerformanceResourceTiming & { responseStatus?: number }>;
    return entries[entries.length - 1]?.responseStatus ?? 0;
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

    if (isCapacitySignal(data.stream_connections, data.max_stream_connections, false)) {
        controller.showCapacityState();
        return;
    }

    if (!data.camera_ready && (data.capture_failures_consecutive || 0) > 3) {
        controller.setPlaceholderPose("worried");
        return;
    }

    if (!data.camera_ready || (data.error_rate_percent || 0) > 20 || (data.capture_failures_consecutive || 0) > 0) {
        controller.setPlaceholderPose("concerned");
        return;
    }

    if ((data.stream_connections || 0) > 0) {
        controller.setPlaceholderPose("curious");
        return;
    }

    if ((data.error_rate_percent || 0) > 5) {
        controller.setPlaceholderPose("concerned");
        return;
    }

    controller.setPlaceholderPose("happy");
}

function openDiagnosticsModal(): void {
    const modal = getElement<HTMLDialogElement>("diagnostics-modal");
    const closeButton = getElement<HTMLButtonElement>("diagnostics-close");
    if (!openNativeDialog(modal, closeButton)) return;

    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
        controller.beginDiagnostics();
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
        controller.endDiagnostics();
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
            const cameraCard = renderStatusMetricCard("Camera", cameraStatus, cameraTone);
            const healthCard = renderStatusMetricCard("System health", data.health_status || "Unknown", healthTone);
            const totalFailures = Number(data.capture_failures_total || 0);
            const noticeTone = healthTone === "neutral" ? "info" : healthTone;
            const existingDetails = content.querySelector<HTMLDetailsElement>(".diagnostics-details");
            const detailsWereOpen = existingDetails?.open ?? false;
            const detailsHadFocus = existingDetails?.contains(document.activeElement) ?? false;

            content.innerHTML = `
                ${renderSectionHeading("Camera health", "camera")}
                <div class="diagnostic-summary-grid">
                    ${cameraCard}
                    ${healthCard}
                </div>

                ${renderNotice(noticeTone, "System status", data.message || "No status details right now.", noticeTone === "error" ? "alert" : "info")}

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

                <div class="refresh-indicator">${renderIcon("refresh")}<span>Refreshes every 3 seconds</span></div>
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
                controller.setPlaceholderPose("concerned");
            }
            const content = getElement<HTMLDivElement>("diagnostics-content");
            content.innerHTML = `<div class="notice notice--error" role="alert">MIO couldn't load diagnostics just now. Please try again in a moment.</div>`;
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
