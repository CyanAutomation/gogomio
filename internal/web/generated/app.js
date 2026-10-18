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

  // internal/web/src/diagnostics-dialog.ts
  var FOCUSABLE_SELECTOR = 'button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
  function focusableElements(modal) {
    return Array.from(modal.querySelectorAll(FOCUSABLE_SELECTOR)).filter((element) => {
      const control = element;
      return !control.disabled && !element.hidden && element.tabIndex >= 0;
    });
  }
  function rememberReturnFocus(documentRef) {
    const activeElement = documentRef.activeElement;
    if (activeElement && "focus" in activeElement && typeof activeElement.focus === "function") {
      return activeElement;
    }
    return null;
  }
  function restoreReturnFocus(element) {
    if (element && element.isConnected) {
      element.focus();
    }
  }
  function handleKeydown(event, modal, closeModal, documentRef) {
    if (modal.hidden) return;
    if (event.key === "Escape") {
      event.preventDefault();
      closeModal();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = focusableElements(modal);
    if (focusable.length === 0) {
      event.preventDefault();
      return;
    }
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const focusIsOutsideDialog = !modal.contains(documentRef.activeElement);
    if (focusIsOutsideDialog || (event.shiftKey ? documentRef.activeElement === first : documentRef.activeElement === last)) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    }
  }

  // internal/web/src/stream-retry.ts
  function nextStreamRetryCount(current, maxRetries) {
    return current < maxRetries ? current + 1 : null;
  }
  function shouldRunStreamRetry(isStreaming, scheduledRetryCount, currentRetryCount) {
    return !isStreaming && scheduledRetryCount === currentRetryCount;
  }

  // internal/web/src/app.ts
  function getElement(id) {
    const element = document.getElementById(id);
    if (!element) throw new Error(`Required dashboard element #${id} was not found`);
    return element;
  }
  var iconMarkup = (name) => `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${name}"></use></svg>`;
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
  function diagnosticMetric(label, value, icon, valueClass = "") {
    return `<div class="metric-row">
        <span class="metric-row__label">${iconMarkup(icon)}<span>${label}</span></span>
        <span class="metric-row__value ${valueClass}">${escapeHTML(value)}</span>
    </div>`;
  }
  var StreamController = class {
    constructor() {
      this.isStreaming = false;
      this.statsInterval = null;
      this.streamAbortController = null;
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
      this.statusIndicator = getElement("status-indicator");
      this.statusText = getElement("status-text");
      this.startButton = getElement("start-stream");
      this.stopButton = getElement("stop-stream");
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
          src: "/static/mio/mio_pose_angry.png?v=45102660b30cef3f208fd7d2663fc9064f421c69dc0a165837c9c52408acc52b",
          alt: "MIO is distressed",
          animationClass: "mio-pulse"
        },
        looking: {
          src: "/static/mio/mio_pose_looking.png?v=004b22d5f48e070762791074142e83cd84314f158005aa5ebf35a5cf64ff9ea6",
          alt: "MIO is scanning",
          animationClass: "mio-float"
        }
      };
      this.initializeStream();
      this.attachEventListeners();
      this.loadStats();
    }
    getPoseForState(state) {
      return this.poseByState[state] || this.poseByState.idle;
    }
    applyPoseToImage(imageElement, state) {
      const pose = this.getPoseForState(state);
      imageElement.src = pose.src;
      imageElement.alt = pose.alt;
      imageElement.className = `mio-state-image ${pose.animationClass}`;
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
        const diagnosticsOpen = !diagnosticsModal.hidden;
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
        this.updateStreamControls();
        this.updateStatus("Streaming", true);
        this.startStatsPolling();
      };
      this.imageElement.onerror = () => {
        this.isStreaming = false;
        this.imageElement.removeAttribute("src");
        this.loadingElement.classList.remove("is-hidden");
        this.imageElement.classList.add("is-hidden");
        this.setIdlePose("warning");
        const nextRetryCount = nextStreamRetryCount(this.streamRetryCount, this.maxStreamRetries);
        if (nextRetryCount !== null) {
          this.streamRetryCount = nextRetryCount;
          this.placeholderText.textContent = "Connection lost";
          this.placeholderDesc.textContent = `Retrying (${this.streamRetryCount}/${this.maxStreamRetries})...`;
          this.showMessage(`Stream reconnecting... (Attempt ${this.streamRetryCount}/${this.maxStreamRetries})`, "warning");
          this.setLoadingPose("running");
          const scheduledRetryCount = nextRetryCount;
          window.setTimeout(() => {
            if (shouldRunStreamRetry(this.isStreaming, scheduledRetryCount, this.streamRetryCount)) {
              this.imageElement.src = "/stream.mjpg?t=" + Date.now();
            }
          }, 2e3);
        } else {
          this.placeholderText.textContent = "Stream unavailable";
          this.placeholderDesc.textContent = "Max retries exceeded. Click Start to try again.";
          this.showMessage("Stream connection failed after multiple attempts. Please check the server is running.", "error");
        }
        this.updateStreamControls();
        this.updateStatus("Disconnected", false);
        this.stopStatsPolling();
        getElement("stat-connections").textContent = "0";
      };
      this.updateStreamControls();
    }
    updateStatus(text, isActive) {
      this.statusText.textContent = text;
      this.statusIndicator.className = isActive ? "status-indicator active" : "status-indicator inactive";
    }
    attachEventListeners() {
      this.startButton.addEventListener("click", () => this.startStream());
      this.stopButton.addEventListener("click", () => this.stopStream());
    }
    startStream() {
      if (this.isStreaming) return;
      this.placeholderText.textContent = "Connecting to stream...";
      this.placeholderDesc.textContent = "Establishing connection...";
      this.loadingElement.classList.remove("is-hidden");
      this.setLoadingPose("loading");
      this.clearPoseResetTimer();
      this.imageElement.classList.add("is-hidden");
      this.updateStatus("Starting...", false);
      this.streamRetryCount = 0;
      this.streamAbortController = new AbortController();
      this.imageElement.src = "/stream.mjpg?t=" + Date.now();
    }
    stopStream() {
      fetch("/api/stream/stop", { method: "POST" }).catch((err) => console.error("Stop stream error:", err));
      if (this.streamAbortController) {
        this.streamAbortController.abort();
        this.streamAbortController = null;
      }
      this.imageElement.removeAttribute("src");
      this.isStreaming = false;
      this.streamRetryCount = 0;
      this.imageElement.classList.add("is-hidden");
      this.loadingElement.classList.remove("is-hidden");
      this.setIdlePose("idle");
      this.clearPoseResetTimer();
      this.placeholderText.textContent = "Stream stopped";
      this.placeholderDesc.textContent = "Select Start Stream to begin viewing.";
      this.updateStreamControls();
      this.updateStatus("Stopped", false);
      this.stopStatsPolling();
      getElement("stat-connections").textContent = "0";
    }
    updateStreamControls() {
      this.startButton.disabled = this.isStreaming;
      this.stopButton.disabled = !this.isStreaming;
    }
    startStatsPolling() {
      if (this.statsInterval !== null) return;
      this.statsInterval = window.setInterval(() => {
        this.loadStats();
      }, 2e3);
    }
    stopStatsPolling() {
      if (this.statsInterval === null) return;
      window.clearInterval(this.statsInterval);
      this.statsInterval = null;
    }
    async loadStats() {
      try {
        const response = await fetch("/api/config");
        if (!response.ok) throw new Error("Failed to load stats");
        const config = await response.json();
        const aspectRatio = resolutionToAspectRatio(config.resolution);
        if (aspectRatio) {
          getElement("stream-container").style.aspectRatio = aspectRatio;
        }
        getElement("feed-mode-badge").hidden = config.mock_camera !== true;
        const fps = config.fps || 0;
        const targetFps = config.target_fps || fps || 30;
        const fpsEl = getElement("stat-fps");
        fpsEl.textContent = fps ? String(fps) : "--";
        fpsEl.classList.remove("metric-card__value--good", "metric-card__value--warning", "metric-card__value--error");
        if (fps) {
          const ratio = fps / targetFps;
          const tone = ratio >= 0.9 ? "good" : ratio >= 0.5 ? "warning" : "error";
          fpsEl.classList.add(`metric-card__value--${tone}`);
        }
        getElement("stat-resolution").textContent = config.resolution ? `${config.resolution[0]}x${config.resolution[1]}` : "--";
        getElement("stat-quality").textContent = (config.jpeg_quality || "--") + "%";
        getElement("stat-connections").textContent = String(config.current_stream_connections);
      } catch (error) {
        console.error("Error loading stats:", error);
      }
    }
    showMessage(text, type = "info") {
      const msg = getElement("message");
      msg.textContent = text;
      msg.className = `message show ${type}`;
      if (!this.isStreaming) {
        if (type === "success") {
          this.setIdlePose("success");
          this.schedulePoseResetToIdle(2200);
        } else if (type === "error") {
          this.setIdlePose("error");
          this.schedulePoseResetToIdle(5e3);
        } else if (type === "warning") {
          this.setIdlePose("warning");
          this.schedulePoseResetToIdle(3e3);
        }
      }
      window.setTimeout(() => {
        msg.classList.add("show");
      }, 10);
      window.setTimeout(() => {
        msg.classList.remove("show");
      }, 4e3);
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
  var diagnosticsReturnFocus = null;
  function openDiagnosticsModal() {
    const modal = getElement("diagnostics-modal");
    if (!modal.hidden) return;
    diagnosticsReturnFocus = rememberReturnFocus(document);
    modal.hidden = false;
    modal.classList.add("is-open");
    document.body.classList.add("modal-open");
    getElement("diagnostics-close").focus();
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
      controller.setLoadingPose("looking");
    }
    fetchDiagnostics();
    if (diagnosticsRefreshInterval !== null) window.clearInterval(diagnosticsRefreshInterval);
    diagnosticsRefreshInterval = window.setInterval(fetchDiagnostics, 3e3);
  }
  function closeDiagnosticsModal() {
    const modal = getElement("diagnostics-modal");
    if (modal.hidden) return;
    modal.classList.remove("is-open");
    modal.hidden = true;
    document.body.classList.remove("modal-open");
    if (diagnosticsRefreshInterval !== null) {
      window.clearInterval(diagnosticsRefreshInterval);
      diagnosticsRefreshInterval = null;
    }
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
      controller.setIdlePose("idle");
    }
    restoreReturnFocus(diagnosticsReturnFocus);
    diagnosticsReturnFocus = null;
  }
  function fetchDiagnostics() {
    fetch("/api/diagnostics").then((response) => {
      if (!response.ok) throw new Error("Failed to load diagnostics");
      return response.json();
    }).then((data) => {
      setCompanionPoseFromDiagnostics(data);
      const content = getElement("diagnostics-content");
      const errorRate = Number(data.error_rate_percent || 0);
      const consecutiveFailures = Number(data.capture_failures_consecutive || 0);
      const healthTone = !data.camera_ready || errorRate > 20 ? "error" : errorRate > 5 || consecutiveFailures > 0 ? "warning" : "success";
      const cameraTone = data.camera_ready ? "success" : "error";
      const cameraStatus = data.camera_ready ? "Ready" : "Unavailable";
      const cameraCard = `<div class="metric-card metric-card--status metric-card--${cameraTone}">
                <div class="metric-card__label">Camera</div>
                <div class="metric-card__value">${cameraStatus}</div>
            </div>`;
      const healthCard = `<div class="metric-card metric-card--status metric-card--${healthTone}">
                <div class="metric-card__label">Health</div>
                <div class="metric-card__value">${escapeHTML(data.health_status || "Unknown")}</div>
            </div>`;
      const totalFailures = Number(data.capture_failures_total || 0);
      const noticeTone = healthTone;
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
                        <div class="metric-card__label">${iconMarkup("alert")} Error Rate</div>
                        <div class="metric-card__value metric-card__value--${healthTone}">${errorRate.toFixed(1)}%</div>
                    </div>
                </div>

                <section class="diagnostics-details" aria-labelledby="diagnostics-metrics-title">
                    <h3 class="diagnostics-subtitle" id="diagnostics-metrics-title">Detailed Metrics</h3>
                    <div class="diagnostic-metric-list">
                        ${diagnosticMetric("Frame Rate", `${Number(data.fps_current || 0).toFixed(1)} fps`, "camera")}
                        ${diagnosticMetric("Resolution", data.resolution || "Unknown", "grid")}
                        ${diagnosticMetric("JPEG Quality", `${Number(data.jpeg_quality || 0)}%`, "quality")}
                        ${diagnosticMetric("Frames Captured", Number(data.frames_captured || 0).toLocaleString(), "image")}
                        ${diagnosticMetric("Last Frame Age", `${Number(data.last_frame_age_seconds || 0).toFixed(2)}s ago`, "clock")}
                        ${diagnosticMetric("Active Streams", `${Number(data.stream_connections || 0)}/${Number(data.max_stream_connections || 0)}`, "link")}
                        ${diagnosticMetric("Recent Failures", consecutiveFailures, "alert", consecutiveFailures > 0 ? "metric-card__value--error" : "")}
                        ${diagnosticMetric("Total Failures", totalFailures, "chart")}
                    </div>
                </section>

                <div class="diagnostic-notice diagnostic-notice--${noticeTone}">
                    <strong>${iconMarkup(noticeTone === "error" ? "alert" : "info")} System Status</strong>
                    <p>${escapeHTML(data.message || "No status details are available.")}</p>
                </div>

                <div class="refresh-indicator">${iconMarkup("refresh")}<span>Updates every 3 seconds</span></div>
            `;
    }).catch((error) => {
      const controller = getStreamController();
      if (controller && !controller.isStreaming) {
        controller.setIdlePose("warning");
      }
      const content = getElement("diagnostics-content");
      const detail = error instanceof Error ? error.message : error;
      content.innerHTML = `<div class="message show error">Diagnostics are unavailable. ${escapeHTML(detail)}</div>`;
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
  document.addEventListener("keydown", (event) => {
    const modal = getElement("diagnostics-modal");
    handleKeydown(event, modal, closeDiagnosticsModal, document);
  });
  getElement("diagnostics-modal").addEventListener("click", (event) => {
    if (event.target === event.currentTarget) closeDiagnosticsModal();
  });
  document.addEventListener("DOMContentLoaded", () => {
    window.streamController = new StreamController();
    getElement("diagnostics-btn").addEventListener("click", openDiagnosticsModal);
    getElement("diagnostics-close").addEventListener("click", closeDiagnosticsModal);
  });
})();
