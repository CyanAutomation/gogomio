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

  // internal/web/src/mio-assets.generated.ts
  var MIO_SPRITES = {
    angry: "/static/mio/mio_pose_angry.png?v=d88bd3c560f5baab73d16afae28d5da29d29ad72ef83671c0266baf517e4858e",
    concerned: "/static/mio/mio_pose_concerned.png?v=c283432edb43252534ab0d04c598b170d2807eceec3f1904380e87a6b567b70f",
    curious: "/static/mio/mio_pose_curious.png?v=a7b931161a9eedd5a759320d647ef0086deb2dfc36739b10d5a4c98267993245",
    happy: "/static/mio/mio_pose_happy.png?v=78ac07e44e305c898bef397e3c29e09fd2cde607af7f3d5ec51315792b5b42e6",
    idle: "/static/mio/mio_pose_idle.png?v=7bd01eb64fed16093f87887065805f47e3b3253c7f8567b5e5ff22101007ca9f",
    looking: "/static/mio/mio_pose_looking.png?v=8e73798b1692de8a45a19c6c52cac8279b497c7e55f6129fec49cfcb305ba70e",
    sleeping: "/static/mio/mio_pose_sleeping.png?v=8a3a7094a658c0cc67844347d51b5158d3f94f381204b4d8bee1c5f78af4cf27",
    worried: "/static/mio/mio_pose_worried.png?v=30c3693ae69272cba7ce477a42ca085ed6483d55f17a919f31b73c83023fc125"
  };

  // internal/web/src/dashboard-presentation.ts
  var MIO_SLEEP_AFTER_MS = 2 * 60 * 1e3;
  var streamStatuses = {
    ready: { label: "Ready", tone: "neutral" },
    connecting: { label: "Connecting\u2026", tone: "warning" },
    live: { label: "Live", tone: "success" },
    reconnecting: { label: "Reconnecting\u2026", tone: "warning" },
    unavailable: { label: "Unavailable", tone: "error" },
    capacity: { label: "At capacity", tone: "warning" }
  };
  function getStreamStatusPresentation(state) {
    return streamStatuses[state];
  }
  function getStreamDescription(state, retryCount = 1, maxRetries = 3) {
    switch (state) {
      case "ready":
        return "MIO's ready when you are.";
      case "connecting":
        return "MIO is warming things up\u2026";
      case "live":
        return "MIO is happily watching your stream.";
      case "reconnecting":
        return `MIO is reconnecting (${Math.max(1, retryCount)} of ${maxRetries})\u2026`;
      case "unavailable":
        return "MIO couldn't reconnect. Check the camera connection, then try again.";
      case "capacity":
        return "MIO's at capacity right now. Try again when another stream is free.";
    }
  }
  var mioPoses = {
    idle: { src: MIO_SPRITES.idle, alt: "MIO is ready", animationClass: "mio-float" },
    sleeping: { src: MIO_SPRITES.sleeping, alt: "MIO is sleeping", animationClass: "mio-float" },
    curious: { src: MIO_SPRITES.curious, alt: "MIO is curious", animationClass: "mio-pulse" },
    concerned: { src: MIO_SPRITES.concerned, alt: "MIO is concerned", animationClass: "mio-pulse" },
    happy: { src: MIO_SPRITES.happy, alt: "MIO is happily watching", animationClass: "mio-float" },
    worried: { src: MIO_SPRITES.worried, alt: "MIO is worried", animationClass: "mio-float" },
    angry: { src: MIO_SPRITES.angry, alt: "MIO is looking out for stream capacity", animationClass: "mio-float" },
    looking: { src: MIO_SPRITES.looking, alt: "MIO is scanning", animationClass: "mio-float" }
  };
  function getMioPosePresentation(pose) {
    return mioPoses[pose];
  }
  function getCompanionPoseForFrameRateTone(tone) {
    if (tone === "success") return "happy";
    if (tone === "error") return "worried";
    return "concerned";
  }
  var ConsecutivePollHysteresis = class {
    constructor(initialValue, minimumConsecutive = 2) {
      this.minimumConsecutive = minimumConsecutive;
      this.candidate = null;
      this.consecutiveCount = 0;
      if (!Number.isInteger(minimumConsecutive) || minimumConsecutive < 1) {
        throw new RangeError("minimumConsecutive must be a positive integer");
      }
      this.currentValue = initialValue;
    }
    get value() {
      return this.currentValue;
    }
    update(nextValue) {
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
    reset(value) {
      this.currentValue = value;
      this.candidate = null;
      this.consecutiveCount = 0;
    }
  };
  function isMioSleepDue(idleSince, now) {
    return Number.isFinite(idleSince) && Number.isFinite(now) && now - idleSince >= MIO_SLEEP_AFTER_MS;
  }
  function isCapacitySignal(activeConnections, maximumConnections, isStreaming, responseStatus = 0) {
    if (responseStatus === 429) return true;
    return !isStreaming && Number.isFinite(activeConnections) && Number.isFinite(maximumConnections) && maximumConnections > 0 && activeConnections >= maximumConnections;
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
  function getCameraReadinessState(responseOk, status) {
    if (!responseOk) return "unavailable";
    if (status === "ready" || status === "initializing") return status;
    return "unavailable";
  }
  function getStreamControlPresentation(state) {
    const active = state === "connecting" || state === "live" || state === "reconnecting";
    return {
      startHidden: active,
      startLabel: state === "unavailable" ? "Retry stream" : state === "capacity" ? "Try again" : "Start stream",
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
  function renderIcon(name) {
    return `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${name}"></use></svg>`;
  }
  function renderSectionHeading(title, icon) {
    return `<div class="section-heading"><h3 class="section-title">${renderIcon(icon)}<span>${escapeHTML(title)}</span></h3></div>`;
  }
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
  function renderStatusMetricCard(label, status, tone) {
    return `<div class="metric-card metric-card--status"><div class="metric-card__label">${escapeHTML(label)}</div>${renderStatusBadge(status, tone)}</div>`;
  }
  function renderNotice(tone, title, message, icon) {
    return `<div class="notice notice--${tone} diagnostic-notice"><strong>${renderIcon(icon)} ${escapeHTML(title)}</strong><p>${escapeHTML(message)}</p></div>`;
  }
  function renderMetricCard(label, value, tone = "neutral") {
    return `<div class="metric-card"><div class="metric-card__label">${escapeHTML(label)}</div><div class="metric-card__value metric-card__value--${tone}">${escapeHTML(value)}</div></div>`;
  }
  function renderMetricRow(label, value, icon, tone = "neutral") {
    const toneClass = tone === "error" ? " metric-card__value--error" : "";
    return `<div class="metric-row"><span class="metric-row__label">${renderIcon(icon)}<span>${escapeHTML(label)}</span></span><span class="metric-row__value${toneClass}">${escapeHTML(value)}</span></div>`;
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
  function diagnosticMetric(label, value, icon, tone = "neutral") {
    return renderMetricRow(label, value, icon, tone);
  }
  var StreamController = class {
    constructor() {
      this.streaming = false;
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
      this.sleepTimer = null;
      this.idleSince = Date.now();
      this.placeholderPose = "idle";
      this.viewerPoseHysteresis = new ConsecutivePollHysteresis("happy", 2);
      this.preloadedSprites = [];
      this.imageElement = getElement("stream-img");
      this.loadingElement = getElement("stream-loading");
      this.placeholderText = getElement("stream-placeholder-text");
      this.placeholderDesc = getElement("stream-placeholder-desc");
      this.placeholderImage = getElement("stream-placeholder-mio");
      this.sleepDecoration = getElement("stream-sleep-z");
      this.headerMascot = getElement("header-mio");
      this.footerMascot = getElement("footer-mio");
      this.viewerMascot = getElement("stream-companion");
      this.liveSparkle = getElement("stream-live-sparkle");
      this.pageDescription = getElement("page-description");
      this.fpsValue = getElement("stat-fps");
      this.fpsIdle = getElement("stat-fps-idle");
      this.fpsMascot = getElement("fps-idle-mio");
      this.streamStatusBadge = getElement("stream-status-badge");
      this.statusText = getElement("status-text");
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
    get isStreaming() {
      return this.streaming;
    }
    preloadSpritesAfterFirstPaint() {
      window.requestAnimationFrame(() => {
        window.setTimeout(() => {
          for (const pose of ["idle", "sleeping", "curious", "concerned", "happy", "worried", "angry", "looking"]) {
            const image = new Image();
            image.decoding = "async";
            image.src = getMioPosePresentation(pose).src;
            this.preloadedSprites.push(image);
          }
        }, 200);
      });
    }
    setPlaceholderPose(pose) {
      this.placeholderPose = pose;
      updateMascotImage(this.placeholderImage, getMioPosePresentation(pose), "placeholder", true);
      this.sleepDecoration.hidden = pose !== "sleeping";
      this.loadingElement.classList.toggle("is-sleeping", pose === "sleeping");
    }
    setViewerPose(pose) {
      updateMascotImage(this.viewerMascot, getMioPosePresentation(pose), "viewer", true);
    }
    setViewerPoseFromTone(tone) {
      const nextPose = getCompanionPoseForFrameRateTone(tone);
      this.setViewerPose(this.viewerPoseHysteresis.update(nextPose));
    }
    clearInactivityTimer() {
      if (this.sleepTimer === null) return;
      window.clearTimeout(this.sleepTimer);
      this.sleepTimer = null;
    }
    scheduleInactivitySleep() {
      this.clearInactivityTimer();
      if (this.streaming || this.streamState !== "ready") return;
      this.idleSince = Date.now();
      this.sleepTimer = window.setTimeout(() => {
        this.sleepTimer = null;
        if (this.streamState !== "ready" || this.streaming || getElement("diagnostics-modal").open || !isMioSleepDue(this.idleSince, Date.now())) return;
        this.setPlaceholderPose("sleeping");
        this.placeholderText.textContent = "MIO's taking a little rest.";
        this.placeholderDesc.textContent = "Start the stream whenever you're ready.";
      }, MIO_SLEEP_AFTER_MS);
    }
    handleUserActivity() {
      if (this.streamState !== "ready") return;
      if (this.placeholderPose === "sleeping") {
        this.setPlaceholderPose("idle");
        this.placeholderText.textContent = "MIO's ready when you are.";
        this.placeholderDesc.textContent = "Start the stream to see what's happening.";
      }
      this.scheduleInactivitySleep();
    }
    initializeStream() {
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
          }, 2e3);
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
    playLiveSparkleOnce() {
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
      this.liveSparkle.classList.remove("is-sparkling");
      void this.liveSparkle.offsetWidth;
      this.liveSparkle.classList.add("is-sparkling");
    }
    showCapacityState() {
      this.streaming = false;
      this.streamRetryCount = 0;
      this.placeholderText.textContent = "MIO's at capacity right now.";
      this.placeholderDesc.textContent = "Try again when another stream is free.";
      this.setPlaceholderPose("angry");
      this.updateStatus("capacity");
      this.updateStreamControls();
      this.updateFPSVisibility();
    }
    updateStatus(state) {
      this.streamState = state;
      const presentation = getStreamStatusPresentation(state);
      updateStatusBadge(
        { root: this.streamStatusBadge, label: this.statusText },
        presentation.label,
        presentation.tone
      );
      this.pageDescription.textContent = getStreamDescription(state, Math.max(1, this.streamRetryCount), this.maxStreamRetries);
      this.updateFPSVisibility();
      if (state !== "ready") this.clearInactivityTimer();
    }
    restorePoseAfterDiagnostics() {
      if (this.streaming) return;
      if (this.streamState === "capacity") this.setPlaceholderPose("angry");
      else if (this.streamState === "unavailable") this.setPlaceholderPose("worried");
      else if (this.streamState === "reconnecting") this.setPlaceholderPose("concerned");
      else if (this.streamState === "connecting") this.setPlaceholderPose("curious");
      else if (this.placeholderPose !== "sleeping") this.setPlaceholderPose("idle");
      this.scheduleInactivitySleep();
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
        const data = response.ok ? await response.json() : void 0;
        this.updateCameraReadiness(getCameraReadinessState(response.ok, data?.status));
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
    beginDiagnostics() {
      if (!this.streaming) this.setPlaceholderPose("looking");
    }
    endDiagnostics() {
      this.restorePoseAfterDiagnostics();
    }
    attachEventListeners() {
      this.startButton.addEventListener("click", () => this.startStream());
      this.stopButton.addEventListener("click", () => this.stopStream());
      document.addEventListener("pointerdown", () => this.handleUserActivity());
      document.addEventListener("keydown", () => this.handleUserActivity());
    }
    startStream() {
      if (this.streaming) return;
      this.clearInactivityTimer();
      this.placeholderText.textContent = "MIO is warming things up\u2026";
      this.placeholderDesc.textContent = "Your camera stream should be ready in a moment.";
      this.loadingElement.classList.remove("is-hidden");
      this.setPlaceholderPose("curious");
      this.imageElement.classList.add("is-hidden");
      this.updateStatus("connecting");
      this.updateStreamControls();
      this.streamRetryCount = 0;
      this.imageElement.src = "/stream.mjpg?t=" + Date.now();
    }
    stopStream() {
      requestStopStream().then(() => this.loadStats()).catch((err) => console.error("Stop stream error:", err));
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
      this.statsInterval = startDashboardPoller(!document.hidden, this.streaming, 2e3, () => {
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
          config.max_stream_connections
        );
        if (isCapacitySignal(metrics.stream_connections, config.max_stream_connections, this.streaming) && this.streamState === "ready") {
          this.showCapacityState();
        } else if (this.streamState === "capacity" && !isCapacitySignal(metrics.stream_connections, config.max_stream_connections, this.streaming)) {
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
    updateFPSVisibility() {
      const live = this.streamState === "live";
      this.fpsValue.hidden = !live;
      this.fpsIdle.hidden = live;
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
  function getImageResponseStatus(source) {
    const absoluteSource = new URL(source, window.location.href).href;
    const entries = performance.getEntriesByName(absoluteSource, "resource");
    return entries[entries.length - 1]?.responseStatus ?? 0;
  }
  var diagnosticsRefreshInterval = null;
  function getStreamController() {
    return window.streamController || null;
  }
  function setCompanionPoseFromDiagnostics(data) {
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
  function openDiagnosticsModal() {
    const modal = getElement("diagnostics-modal");
    const closeButton = getElement("diagnostics-close");
    if (!openNativeDialog(modal, closeButton)) return;
    const controller = getStreamController();
    if (controller && !controller.isStreaming) {
      controller.beginDiagnostics();
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
      controller.endDiagnostics();
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
      const cameraCard = renderStatusMetricCard("Camera", cameraStatus, cameraTone);
      const healthCard = renderStatusMetricCard("System health", data.health_status || "Unknown", healthTone);
      const totalFailures = Number(data.capture_failures_total || 0);
      const noticeTone = healthTone === "neutral" ? "info" : healthTone;
      const existingDetails = content.querySelector(".diagnostics-details");
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
      const updatedDetails = content.querySelector(".diagnostics-details");
      if (updatedDetails) {
        updatedDetails.open = detailsWereOpen;
        if (detailsHadFocus) updatedDetails.querySelector("summary")?.focus();
      }
    }).catch((error) => {
      console.error("Diagnostics request failed:", error);
      const controller = getStreamController();
      if (controller && !controller.isStreaming) {
        controller.setPlaceholderPose("concerned");
      }
      const content = getElement("diagnostics-content");
      content.innerHTML = `<div class="notice notice--error" role="alert">MIO couldn't load diagnostics just now. Please try again in a moment.</div>`;
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
