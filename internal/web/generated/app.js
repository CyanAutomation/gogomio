"use strict";
(() => {
  // internal/web/src/aspect-ratio.ts
  function resolutionToAspectRatio(resolution) {
    if (!Array.isArray(resolution) || resolution.length < 2) return null;
    const [width, height] = resolution;
    if (typeof width !== "number" || typeof height !== "number" || !Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
      return null;
    }
    return `${width} / ${height}`;
  }

  // internal/web/src/api.ts
  function requestCameraConfig(fetcher = fetch) {
    return fetcher("/v1/config/camera");
  }
  function requestCameraReadiness(fetcher = fetch) {
    return fetcher("/v1/ready", { cache: "no-store" });
  }
  function requestLiveMetrics(fetcher = fetch) {
    return fetcher("/v1/metrics/live", { cache: "no-store" });
  }
  function requestDiagnostics(fetcher = fetch) {
    return fetcher("/v1/health/detailed", { cache: "no-store" });
  }
  function requestStopStream(fetcher = fetch) {
    return fetcher("/v1/api/stream/stop", { method: "POST" });
  }

  // internal/web/src/dashboard-presentation.ts
  var streamStatuses = {
    ready: { label: "Ready", tone: "neutral" },
    connecting: { label: "Connecting\u2026", tone: "warning" },
    live: { label: "Live", tone: "success" },
    reconnecting: { label: "Reconnecting\u2026", tone: "warning" },
    unavailable: { label: "Stream unavailable", tone: "error" }
  };
  function getStreamStatusPresentation(state) {
    return streamStatuses[state];
  }
  var streamMascots = {
    ready: {
      src: "/static/mio/mio_pose_sleeping.png?v=f7f7212e2ebdf8a6438d93e7d3e4c8a2a7c02a3278d393ba13aad2a7fe040d66",
      alt: "MIO is resting",
      animationClass: "mio-blink"
    },
    connecting: {
      src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
      alt: "MIO is curious",
      animationClass: "mio-pulse"
    },
    live: {
      src: "/static/mio/mio_pose_happy.png?v=e2bf5184a25fad3fdd17320965ecc8bd022126f26379ae7df2a857c71ee265be",
      alt: "MIO is happily watching",
      animationClass: "mio-float"
    },
    reconnecting: {
      src: "/static/mio/mio_pose_concerned.png?v=b28941a29aef2eebe3e74be347dfc9a6e208f61771ec72907e8e7700276f3b5c",
      alt: "MIO is concerned",
      animationClass: "mio-pulse"
    },
    unavailable: {
      src: "/static/mio/mio_pose_worried.png?v=2c136a60a29ff48447aa0bd6484050e7af02efa87cbcab7279db3455215f92e4",
      alt: "MIO is worried",
      animationClass: "mio-float"
    }
  };
  function getStreamMascotPresentation(state) {
    return streamMascots[state];
  }
  function getCameraReadinessPresentation(state) {
    const presentations = {
      checking: { label: "Checking camera\u2026", tone: "info", guidance: "" },
      ready: { label: "Camera ready", tone: "success", guidance: "" },
      initializing: {
        label: "Camera starting",
        tone: "warning",
        guidance: "The camera is still starting. Try again in a moment."
      },
      unavailable: {
        label: "Camera unavailable",
        tone: "error",
        guidance: "Check the camera connection, then try the stream again."
      },
      unknown: {
        label: "Camera status unknown",
        tone: "warning",
        guidance: "Could not check camera status. Check your connection to GoGoMio."
      }
    };
    return presentations[state];
  }
  function getStreamControlPresentation(state) {
    const active = state === "connecting" || state === "live" || state === "reconnecting";
    return {
      startHidden: active,
      startLabel: state === "unavailable" ? "Retry stream" : "Start stream",
      stopHidden: !active,
      stopLabel: state === "connecting" ? "Cancel connection" : "Stop stream",
      stopPrimary: state === "live" || state === "reconnecting"
    };
  }
  function formatFrameRate(frameRate) {
    return Number.isFinite(frameRate) && frameRate >= 0 ? `${frameRate.toFixed(1)} fps` : "--";
  }
  function getFrameRateTone(frameRate, targetFrameRate, isLive) {
    if (!isLive || !Number.isFinite(frameRate) || !Number.isFinite(targetFrameRate) || targetFrameRate <= 0) {
      return "neutral";
    }
    const ratio = frameRate / targetFrameRate;
    return ratio >= 0.9 ? "success" : ratio >= 0.5 ? "warning" : "error";
  }
  function getHealthTone(healthStatus, cameraReady, errorRate, consecutiveFailures) {
    if (!cameraReady || healthStatus === "Poor" || errorRate > 20) return "error";
    if (healthStatus === "Degraded" || errorRate > 5 || consecutiveFailures > 0) return "warning";
    if (healthStatus === "Idle") return "neutral";
    return "success";
  }
  function formatResolution(resolution) {
    if (Array.isArray(resolution)) {
      const [width, height] = resolution;
      return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0 ? `${width} \xD7 ${height}` : "--";
    }
    if (typeof resolution !== "string") return "--";
    const match = resolution.match(/^\s*(\d+)\s*[x×]\s*(\d+)\s*$/i);
    return match ? `${match[1]} \xD7 ${match[2]}` : "--";
  }
  function formatStreamConnections(active, maximum) {
    const activeText = Number.isFinite(active) && active >= 0 ? String(active) : "--";
    const maximumText = Number.isFinite(maximum) && maximum >= 0 ? String(maximum) : "--";
    return `${activeText} / ${maximumText}`;
  }
  function formatFrameAge(ageSeconds) {
    return typeof ageSeconds === "number" && Number.isFinite(ageSeconds) && ageSeconds >= 0 ? `${ageSeconds.toFixed(2)}s ago` : "No frames yet";
  }

  // internal/web/src/diagnostics-dialog.ts
  function openNativeDialog(dialog, initialFocus) {
    if (dialog.open) return false;
    dialog.showModal();
    initialFocus?.focus();
    return true;
  }
  function closeNativeDialog(dialog, returnValue) {
    if (!dialog.open) return false;
    dialog.close(returnValue);
    return true;
  }

  // internal/web/src/stream-retry.ts
  function nextStreamRetryCount(current, maxRetries) {
    return current < maxRetries ? current + 1 : null;
  }
  function shouldRunStreamRetry(isStreaming, scheduledRetryCount, currentRetryCount) {
    return !isStreaming && scheduledRetryCount === currentRetryCount;
  }

  // internal/web/src/polling.ts
  function startDashboardPoller(isVisible, isActive, intervalMs, poll, schedule = (callback, delay) => window.setInterval(callback, delay)) {
    if (!isVisible || !isActive) return null;
    return schedule(poll, intervalMs);
  }

  // internal/web/src/snackbar.ts
  var browserScheduler = {
    set: (callback, delayMs) => window.setTimeout(callback, delayMs),
    clear: (handle) => window.clearTimeout(handle)
  };
  var SnackbarController = class {
    constructor(view, scheduler = browserScheduler, durationMs = 4500) {
      this.view = view;
      this.scheduler = scheduler;
      this.durationMs = durationMs;
      this.timer = null;
      this.message = null;
      this.tone = null;
      this.paused = false;
    }
    show(text, tone = "info") {
      if (this.message !== text || this.tone !== tone) {
        this.message = text;
        this.tone = tone;
        this.view.show(text, tone);
      }
      this.scheduleDismissal();
    }
    pause() {
      if (this.message === null || this.paused) return;
      this.paused = true;
      this.clearTimer();
    }
    resume() {
      if (this.message === null || !this.paused) return;
      this.paused = false;
      this.scheduleDismissal();
    }
    dismiss() {
      this.clearTimer();
      if (this.message !== null) this.view.hide();
      this.message = null;
      this.tone = null;
      this.paused = false;
    }
    scheduleDismissal() {
      this.clearTimer();
      if (this.paused || this.message === null) return;
      this.timer = this.scheduler.set(() => this.dismiss(), this.durationMs);
    }
    clearTimer() {
      if (this.timer === null) return;
      this.scheduler.clear(this.timer);
      this.timer = null;
    }
  };

  // internal/web/src/ui-components.ts
  function updateStatusBadge(view, label, tone) {
    view.root.className = `ui-badge ui-badge--${tone}`;
    if (view.label.textContent !== label) view.label.textContent = label;
  }
  function updateMascotImage(image, presentation, size, decorative = false) {
    image.src = presentation.src;
    image.alt = decorative ? "" : presentation.alt;
    image.className = `mio-avatar mio-avatar--${size} ${presentation.animationClass}`;
  }
  function renderStatusBadge(label, tone) {
    return `<span class="ui-badge ui-badge--${tone}"><span class="ui-badge__dot" aria-hidden="true"></span><span class="ui-badge__label">${escapeHTML(label)}</span></span>`;
  }
  function renderMetricCard(label, value, tone = "neutral") {
    return `<div class="metric-card"><div class="metric-card__label">${escapeHTML(label)}</div><div class="metric-card__value metric-card__value--${tone}">${escapeHTML(value)}</div></div>`;
  }
  function renderMetricRow(label, value, icon, tone = "neutral") {
    const iconMarkup2 = `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${icon}"></use></svg>`;
    const toneClass = tone === "error" ? " metric-card__value--error" : "";
    return `<div class="metric-row"><span class="metric-row__label">${iconMarkup2}<span>${escapeHTML(label)}</span></span><span class="metric-row__value${toneClass}">${escapeHTML(value)}</span></div>`;
  }
  function escapeHTML(value) {
    const entities = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;"
    };
    return String(value ?? "").replace(/[&<>"']/g, (character) => entities[character]);
  }

  // internal/web/src/app.ts
  function getElement(id) {
    const element = document.getElementById(id);
    if (!element) throw new Error(`Required dashboard element #${id} was not found`);
    return element;
  }
  var iconMarkup = (name) => `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${name}"></use></svg>`;
  function diagnosticMetric(label, value, icon, tone = "neutral") {
    return renderMetricRow(label, value, icon, tone);
  }
  var StreamController = class {
    constructor() {
      this.isStreaming = false;
      this.streamState = "ready";
      this.statsInterval = null;
      this.cameraReadinessInterval = null;
      this.cameraReadinessPromise = null;
      this.cameraReadiness = "checking";
      this.cameraConfig = null;
      this.cameraConfigExpiresAt = 0;
      this.cameraConfigPromise = null;
      this.streamRetryCount = 0;
      this.maxStreamRetries = 3;
      this.poseResetTimer = null;
      this.imageElement = getElement("stream-img");
      this.loadingElement = getElement("stream-loading");
      this.placeholderText = getElement("stream-placeholder-text");
      this.placeholderDesc = getElement("stream-placeholder-desc");
      this.spinnerElement = getElement("stream-spinner");
      this.idleElement = getElement("stream-idle");
      this.spinnerImage = getElement("stream-spinner-image");
      this.idleImage = getElement("stream-idle-image");
      this.streamStatusBadge = getElement("stream-status-badge");
      this.statusText = getElement("status-text");
      this.streamStatusMascot = getElement("stream-status-mascot");
      this.cameraStatusBadge = getElement("camera-status-badge");
      this.cameraStatusText = getElement("camera-status-text");
      this.cameraStatusGuidance = getElement("camera-status-guidance");
      this.startButton = getElement("start-stream");
      this.stopButton = getElement("stop-stream");
      const snackbarElement = getElement("snackbar");
      const snackbarText = getElement("snackbar-text");
      this.snackbar = new SnackbarController({
        show: (text, tone) => {
          snackbarText.textContent = text;
          snackbarElement.className = `snackbar notice notice--${tone} is-visible`;
        },
        hide: () => snackbarElement.classList.remove("is-visible")
      });
      snackbarElement.addEventListener("mouseenter", () => this.snackbar.pause());
      snackbarElement.addEventListener("mouseleave", () => {
        if (!snackbarElement.contains(document.activeElement)) this.snackbar.resume();
      });
      snackbarElement.addEventListener("focusin", () => this.snackbar.pause());
      snackbarElement.addEventListener("focusout", (event) => {
        if (!snackbarElement.contains(event.relatedTarget) && !snackbarElement.matches(":hover")) {
          this.snackbar.resume();
        }
      });
      getElement("snackbar-dismiss").addEventListener("click", () => this.snackbar.dismiss());
      this.poseByState = {
        idle: {
          src: "/static/mio/mio_pose_sleeping.png?v=f7f7212e2ebdf8a6438d93e7d3e4c8a2a7c02a3278d393ba13aad2a7fe040d66",
          alt: "MIO is resting",
          animationClass: "mio-blink"
        },
        loading: {
          src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
          alt: "MIO is curious",
          animationClass: "mio-pulse"
        },
        success: {
          src: "/static/mio/mio_pose_happy.png?v=e2bf5184a25fad3fdd17320965ecc8bd022126f26379ae7df2a857c71ee265be",
          alt: "MIO is happy",
          animationClass: "mio-float"
        },
        warning: {
          src: "/static/mio/mio_pose_concerned.png?v=b28941a29aef2eebe3e74be347dfc9a6e208f61771ec72907e8e7700276f3b5c",
          alt: "MIO is concerned",
          animationClass: "mio-pulse"
        },
        running: {
          src: "/static/mio/mio_pose_curious.png?v=acdd90f6293d65657beadd82466dcfa8a39ab308fc8add922925634cfa5b429f",
          alt: "MIO is attentive",
          animationClass: "mio-pulse"
        },
        error: {
          src: "/static/mio/mio_pose_worried.png?v=2c136a60a29ff48447aa0bd6484050e7af02efa87cbcab7279db3455215f92e4",
          alt: "MIO is worried",
          animationClass: "mio-pulse"
        },
        looking: {
          src: "/static/mio/mio_pose_looking.png?v=004b22d5f48e070762791074142e83cd84314f158005aa5ebf35a5cf64ff9ea6",
          alt: "MIO is scanning",
          animationClass: "mio-float"
        }
      };
      this.updateStatus("ready");
      this.updateCameraReadiness("checking");
      this.initializeStream();
      this.attachEventListeners();
      this.loadCameraReadiness();
      this.startCameraReadinessPolling();
      this.loadStats();
    }
    getPoseForState(state) {
      return this.poseByState[state] || this.poseByState.idle;
    }
    applyPoseToImage(imageElement, state) {
      const pose = this.getPoseForState(state);
      imageElement.src = pose.src;
      imageElement.alt = pose.alt;
      imageElement.className = `mio-avatar mio-avatar--state ${pose.animationClass}`;
    }
    setLoadingPose(state = "loading") {
      this.applyPoseToImage(this.spinnerImage, state);
      this.spinnerElement.classList.remove("is-hidden");
      this.idleElement.classList.add("is-hidden");
    }
    setIdlePose(state = "idle") {
      this.applyPoseToImage(this.idleImage, state);
      this.spinnerElement.classList.add("is-hidden");
      this.idleElement.classList.remove("is-hidden");
    }
    clearPoseResetTimer() {
      if (this.poseResetTimer !== null) {
        window.clearTimeout(this.poseResetTimer);
        this.poseResetTimer = null;
      }
    }
    schedulePoseResetToIdle(delayMs = 2200) {
      this.clearPoseResetTimer();
      this.poseResetTimer = window.setTimeout(() => {
        const diagnosticsModal = getElement("diagnostics-modal");
        const diagnosticsOpen = diagnosticsModal.open;
        if (!this.isStreaming && !diagnosticsOpen) {
          this.setIdlePose("idle");
        }
        this.poseResetTimer = null;
      }, delayMs);
    }
    initializeStream() {
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
          this.placeholderDesc.textContent = `MIO is reconnecting (${this.streamRetryCount} of ${this.maxStreamRetries})\u2026`;
          this.updateStatus("reconnecting");
          this.setLoadingPose("warning");
          const scheduledRetryCount = nextRetryCount;
          window.setTimeout(() => {
            if (shouldRunStreamRetry(this.isStreaming, scheduledRetryCount, this.streamRetryCount)) {
              this.imageElement.src = "/stream.mjpg?t=" + Date.now();
            }
          }, 2e3);
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
    updateStatus(state) {
      this.streamState = state;
      const presentation = getStreamStatusPresentation(state);
      updateStatusBadge(
        { root: this.streamStatusBadge, label: this.statusText },
        presentation.label,
        presentation.tone
      );
      updateMascotImage(this.streamStatusMascot, getStreamMascotPresentation(state), "status", true);
    }
    updateCameraReadiness(state) {
      if (this.cameraReadiness === state) return;
      this.cameraReadiness = state;
      const presentation = getCameraReadinessPresentation(state);
      updateStatusBadge(
        { root: this.cameraStatusBadge, label: this.cameraStatusText },
        presentation.label,
        presentation.tone
      );
      this.cameraStatusGuidance.textContent = presentation.guidance;
      this.cameraStatusGuidance.hidden = presentation.guidance.length === 0;
    }
    loadCameraReadiness() {
      if (this.cameraReadinessPromise) return this.cameraReadinessPromise;
      this.cameraReadinessPromise = requestCameraReadiness().then(async (response) => {
        const data = await response.json();
        if (response.ok && data.status === "ready") {
          this.updateCameraReadiness("ready");
        } else if (data.status === "initializing") {
          this.updateCameraReadiness("initializing");
        } else {
          this.updateCameraReadiness("unavailable");
        }
      }).catch(() => this.updateCameraReadiness("unknown")).finally(() => {
        this.cameraReadinessPromise = null;
      });
      return this.cameraReadinessPromise;
    }
    startCameraReadinessPolling() {
      if (this.cameraReadinessInterval !== null) return;
      this.cameraReadinessInterval = startDashboardPoller(
        !document.hidden,
        true,
        5e3,
        () => {
          void this.loadCameraReadiness();
        }
      );
    }
    stopCameraReadinessPolling() {
      if (this.cameraReadinessInterval === null) return;
      window.clearInterval(this.cameraReadinessInterval);
      this.cameraReadinessInterval = null;
    }
    attachEventListeners() {
      this.startButton.addEventListener("click", () => this.startStream());
      this.stopButton.addEventListener("click", () => this.stopStream());
    }
    startStream() {
      if (this.isStreaming) return;
      this.placeholderText.textContent = "MIO is warming things up\u2026";
      this.placeholderDesc.textContent = "Your camera stream should be ready in a moment.";
      this.loadingElement.classList.remove("is-hidden");
      this.setLoadingPose("loading");
      this.clearPoseResetTimer();
      this.imageElement.classList.add("is-hidden");
      this.updateStatus("connecting");
      this.updateStreamControls();
      this.streamRetryCount = 0;
      this.imageElement.src = "/stream.mjpg?t=" + Date.now();
    }
    stopStream() {
      requestStopStream().then(() => this.loadStats()).catch((err) => console.error("Stop stream error:", err));
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
    updateStreamControls() {
      const presentation = getStreamControlPresentation(this.streamState);
      const activeControl = document.activeElement === this.startButton ? "start" : document.activeElement === this.stopButton ? "stop" : null;
      this.startButton.hidden = presentation.startHidden;
      this.startButton.disabled = presentation.startHidden;
      this.startButton.querySelector("span").textContent = presentation.startLabel;
      this.stopButton.hidden = presentation.stopHidden;
      this.stopButton.disabled = presentation.stopHidden;
      this.stopButton.querySelector("span").textContent = presentation.stopLabel;
      this.stopButton.classList.toggle("button--primary", presentation.stopPrimary);
      this.stopButton.classList.toggle("button--secondary", !presentation.stopPrimary);
      if (activeControl === "start" && presentation.startHidden) this.stopButton.focus();
      if (activeControl === "stop" && presentation.stopHidden) this.startButton.focus();
    }
    startStatsPolling() {
      if (this.statsInterval !== null) return;
      this.statsInterval = startDashboardPoller(!document.hidden, this.isStreaming, 2e3, () => {
        this.loadStats();
      });
    }
    stopStatsPolling() {
      if (this.statsInterval === null) return;
      window.clearInterval(this.statsInterval);
      this.statsInterval = null;
    }
    async loadStats() {
      try {
        const [config, metricsResponse] = await Promise.all([
          this.loadCameraConfig(),
          requestLiveMetrics()
        ]);
        if (!metricsResponse.ok) throw new Error("Failed to load live camera metrics");
        const metrics = await metricsResponse.json();
        const aspectRatio = resolutionToAspectRatio(config.resolution);
        if (aspectRatio) {
          getElement("stream-container").style.aspectRatio = aspectRatio;
        }
        getElement("feed-mode-badge").hidden = config.mock_camera !== true;
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
          config.max_stream_connections
        );
      } catch (error) {
        console.error("Error loading stats:", error);
      }
    }
    loadCameraConfig() {
      if (this.cameraConfig && Date.now() < this.cameraConfigExpiresAt) {
        return Promise.resolve(this.cameraConfig);
      }
      if (this.cameraConfigPromise) return this.cameraConfigPromise;
      this.cameraConfigPromise = requestCameraConfig().then(async (response) => {
        if (!response.ok) throw new Error("Failed to load camera configuration");
        const config = await response.json();
        this.cameraConfig = config;
        this.cameraConfigExpiresAt = Date.now() + 6e4;
        return config;
      }).finally(() => {
        this.cameraConfigPromise = null;
      });
      return this.cameraConfigPromise;
    }
  };
  var diagnosticsRefreshInterval = null;
  function getStreamController() {
    return window.streamController || null;
  }
  function setCompanionPoseFromDiagnostics(data) {
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
  function openDiagnosticsModal() {
    const modal = getElement("diagnostics-modal");
    const closeButton = getElement("diagnostics-close");
    if (!openNativeDialog(modal, closeButton)) return;
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
      controller.setLoadingPose("looking");
    }
    startDiagnosticsPolling();
  }
  function closeDiagnosticsModal() {
    const modal = getElement("diagnostics-modal");
    closeNativeDialog(modal);
  }
  function handleDiagnosticsDialogClose() {
    stopDiagnosticsPolling();
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
      controller.setIdlePose("idle");
    }
  }
  function startDiagnosticsPolling() {
    stopDiagnosticsPolling();
    const modal = getElement("diagnostics-modal");
    const interval = startDashboardPoller(!document.hidden, modal.open, 3e3, fetchDiagnostics);
    if (interval === null) return;
    fetchDiagnostics();
    diagnosticsRefreshInterval = interval;
  }
  function stopDiagnosticsPolling() {
    if (diagnosticsRefreshInterval === null) return;
    window.clearInterval(diagnosticsRefreshInterval);
    diagnosticsRefreshInterval = null;
  }
  function handleDashboardVisibilityChange() {
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
    if (getElement("diagnostics-modal").open) {
      startDiagnosticsPolling();
    }
  }
  function fetchDiagnostics() {
    requestDiagnostics().then((response) => {
      if (!response.ok) throw new Error("Failed to load diagnostics");
      return response.json();
    }).then((data) => {
      setCompanionPoseFromDiagnostics(data);
      getStreamController()?.updateCameraReadiness(data.camera_ready ? "ready" : "unavailable");
      const content = getElement("diagnostics-content");
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
      const existingDetails = content.querySelector(".diagnostics-details");
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
      const updatedDetails = content.querySelector(".diagnostics-details");
      if (updatedDetails) {
        updatedDetails.open = detailsWereOpen;
        if (detailsHadFocus) updatedDetails.querySelector("summary")?.focus();
      }
    }).catch((error) => {
      console.error("Diagnostics request failed:", error);
      const controller = getStreamController();
      if (controller && !controller.isStreaming) {
        controller.setIdlePose("warning");
      }
      const content = getElement("diagnostics-content");
      content.innerHTML = `<div class="notice notice--error message show" role="alert">MIO couldn't load diagnostics just now. Please try again in a moment.</div>`;
    });
  }
  function formatUptime(seconds) {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor(seconds % 86400 / 3600);
    const mins = Math.floor(seconds % 3600 / 60);
    const secs = seconds % 60;
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    if (mins > 0) return `${mins}m ${secs}s`;
    return `${secs}s`;
  }
  document.addEventListener("DOMContentLoaded", () => {
    window.streamController = new StreamController();
    getElement("diagnostics-btn").addEventListener("click", openDiagnosticsModal);
    getElement("diagnostics-close").addEventListener("click", closeDiagnosticsModal);
    getElement("diagnostics-modal").addEventListener("close", handleDiagnosticsDialogClose);
    document.addEventListener("visibilitychange", handleDashboardVisibilityChange);
  });
})();
