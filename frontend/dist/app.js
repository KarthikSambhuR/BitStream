const api = () => window.go?.main?.App;

const state = {
  selectedIndex: -1,
  currentTab: 0,
  isRecording: false,
  tick: null,
  previewTick: null,
  previewBusy: false,
  previewFrameRequest: null,
  previewCtx: null,
  nativePreview: false,
  windowSaveTick: null,
};

const devicesState = {
  cameras: [],
  microphones: [],
  selectedCameraId: null,
  selectedMics: [], // Allow multiple selected audio source names
  cameraStream: null,
};

const els = {
  sourceList: document.querySelector("#sourceList"),
  tabs: [...document.querySelectorAll(".tab")],
  refresh: document.querySelector("#refresh"),
  folder: document.querySelector("#folder"),
  record: document.querySelector("#record"),
  previewTitle: document.querySelector("#previewTitle"),
  previewName: document.querySelector("#previewName"),
  previewMeta: document.querySelector("#previewMeta"),
  previewCanvas: document.querySelector("#previewCanvas"),
  previewCard: document.querySelector("#previewPlaceholder"),
  enginePill: document.querySelector("#enginePill"),
  timer: document.querySelector("#timer"),
  recordDot: document.querySelector("#recordDot"),
  recordState: document.querySelector("#recordState"),
  targetMem: document.querySelector("#targetMem"),
  ceilingMem: document.querySelector("#ceilingMem"),
  backend: document.querySelector("#backend"),
  backendDetail: document.querySelector("#backendDetail"),
  minimise: document.querySelector("#minimise"),
  maximise: document.querySelector("#maximise"),
  close: document.querySelector("#close"),
};

function sourceLabel(type) {
  if (type === "screen") return "Display Feed";
  if (type === "camera") return "Camera Feed";
  return "Window Capture";
}

function render(next) {
  state.selectedIndex = next.selectedIndex;
  state.currentTab = next.currentTab;
  state.isRecording = next.isRecording;

  els.tabs.forEach((tab) => {
    tab.classList.toggle("active", Number(tab.dataset.tab) === next.currentTab);
  });

  // Render Screens/Windows sourceList
  els.sourceList.replaceChildren(
    ...next.sources.map((source) => {
      const button = document.createElement("button");
      button.className = `source ${source.index === next.selectedIndex ? "active" : ""}`;
      const iconName = source.type === "screen" ? "monitor" : "layers";
      button.innerHTML = `
        <div class="thumb ${source.type}">
          <i data-lucide="${iconName}"></i>
        </div>
        <div class="source-info">
          <strong title="${escapeHtml(source.name)}">${escapeHtml(source.name)}</strong>
          <span>${sourceLabel(source.type)}</span>
        </div>
      `;
      button.addEventListener("click", async () => {
        render(await api().SelectSource(source.index));
      });
      return button;
    }),
  );

  // Render active selection details
  const selected = next.sources.find((item) => item.index === next.selectedIndex);
  els.previewTitle.textContent = selected ? selected.name : "Select a source";
  els.previewName.textContent = selected ? selected.name : "No source selected";
  els.previewMeta.textContent = selected
    ? `${sourceLabel(selected.type)} armed for capture`
    : "Choose a screen or app window from the left rail";
  els.previewCard.classList.toggle("hidden", Boolean(selected));
  els.previewCanvas.classList.toggle("live", Boolean(selected));

  els.enginePill.textContent = next.ffmpegAvailable
    ? next.isRecording
      ? "Recording"
      : "Ready"
    : "FFmpeg missing";
  els.enginePill.classList.toggle("bad", !next.ffmpegAvailable || next.isRecording);

  els.timer.textContent = next.timer;
  els.recordState.textContent = next.isRecording ? "Capturing" : "Idle";
  els.recordDot.classList.toggle("live", next.isRecording);

  // Update Record Button
  const recordIcon = document.getElementById("recordBtnIcon");
  const recordText = document.getElementById("recordBtnText");
  if (recordIcon && recordText) {
    if (next.isRecording) {
      recordIcon.setAttribute("data-lucide", "square");
      recordText.textContent = "Stop Recording";
      els.record.className = "record-btn stop";
    } else {
      recordIcon.setAttribute("data-lucide", "play");
      recordText.textContent = "Start Recording";
      els.record.className = "record-btn";
    }
  }

  els.targetMem.textContent = next.memoryTargetMb;
  els.ceilingMem.textContent = next.memoryCeilingMb;
  els.backend.textContent = next.captureBackend;
  els.backendDetail.textContent = next.wgcCanCapture
    ? "Selected source is WGC-compatible. Browser capture prompts are disabled; native WGC preview is the next renderer path."
    : next.backendDetail;

  manageTimer(next.isRecording);
  managePreview(state.selectedIndex >= 0, state.selectedIndex);
  renderCameraList();
  renderAudioList();

  refreshIcons();
}

function refreshIcons() {
  if (window.lucide) {
    window.lucide.createIcons();
  }
}

function renderCameraList() {
  const cameraList = document.getElementById("cameraList");
  if (!cameraList) return;

  const items = [
    { label: "No Overlay / Disabled", deviceId: "None", labelText: "Disabled" }
  ];

  devicesState.cameras.forEach((cam) => {
    items.push({
      label: cam.label || "Camera Device",
      deviceId: cam.deviceId,
      labelText: "Camera Stream"
    });
  });

  cameraList.replaceChildren(
    ...items.map(item => {
      const button = document.createElement("button");
      const isActive = devicesState.selectedCameraId === item.deviceId;
      button.className = `camera-item ${isActive ? "active" : ""}`;
      button.type = "button";
      button.setAttribute("role", "radio");
      button.setAttribute("aria-checked", String(isActive));
      button.innerHTML = `
        <div class="camera-icon">
          <i data-lucide="${item.deviceId === "None" ? "video-off" : "camera"}"></i>
        </div>
        <div class="camera-info">
          <strong>${escapeHtml(item.label)}</strong>
          <span>${item.labelText}</span>
        </div>
      `;
      button.addEventListener("click", async () => {
        devicesState.selectedCameraId = item.deviceId;
        if (item.deviceId === "None") {
          stopCameraPreview();
        } else {
          const matchedCam = devicesState.cameras.find(c => c.deviceId === item.deviceId);
          if (matchedCam) {
            await startCameraPreview(matchedCam);
          }
        }
        renderCameraList();
      });
      return button;
    })
  );
  refreshIcons();
}

function renderAudioList() {
  const audioList = document.getElementById("audioList");
  if (!audioList) return;

  const items = [
    { label: "Mute / No Audio", name: "None", icon: "volume-x" },
    { label: "System Playback Loopback", name: "System Loopback", icon: "monitor-play" }
  ];

  devicesState.microphones.forEach((mic, i) => {
    items.push({
      label: mic.label || `Microphone ${i + 1}`,
      name: mic.label || `Microphone ${i + 1}`,
      icon: "mic"
    });
  });

  audioList.replaceChildren(
    ...items.map(item => {
      const button = document.createElement("button");
      const isActive = devicesState.selectedMics.includes(item.name) || (devicesState.selectedMics.length === 0 && item.name === "None");
      button.className = `audio-item ${isActive ? "active" : ""}`;
      button.type = "button";
      button.setAttribute("role", "checkbox");
      button.setAttribute("aria-checked", String(isActive));
      button.innerHTML = `
        <div class="audio-icon">
          <i data-lucide="${isActive ? 'check-square' : 'square'}"></i>
        </div>
        <div class="audio-info">
          <strong>${escapeHtml(item.label)}</strong>
        </div>
      `;
      button.addEventListener("click", () => {
        if (item.name === "None") {
          devicesState.selectedMics = ["None"];
        } else {
          // Remove "None" if other input is selected
          devicesState.selectedMics = devicesState.selectedMics.filter(m => m !== "None");
          
          if (devicesState.selectedMics.includes(item.name)) {
            devicesState.selectedMics = devicesState.selectedMics.filter(m => m !== item.name);
          } else {
            devicesState.selectedMics.push(item.name);
          }
          if (devicesState.selectedMics.length === 0) {
            devicesState.selectedMics = ["None"];
          }
        }
        renderAudioList();
      });
      return button;
    })
  );
  refreshIcons();
}

async function startCameraPreview(camera) {
  stopCameraPreview();

  const container = document.getElementById("previewCameraContainer");
  const videoEl = document.getElementById("previewVideo");
  if (!container || !videoEl) return;

  container.classList.remove("hidden");

  try {
    const stream = await navigator.mediaDevices.getUserMedia({
      video: { deviceId: { exact: camera.deviceId } }
    });
    devicesState.cameraStream = stream;
    videoEl.srcObject = stream;
    videoEl.play();
  } catch (err) {
    console.error("Failed to start camera overlay preview:", err);
  }
}

function stopCameraPreview() {
  const container = document.getElementById("previewCameraContainer");
  const videoEl = document.getElementById("previewVideo");
  if (videoEl) {
    videoEl.srcObject = null;
  }
  if (container) {
    container.classList.add("hidden");
  }
  if (devicesState.cameraStream) {
    devicesState.cameraStream.getTracks().forEach((track) => track.stop());
    devicesState.cameraStream = null;
  }
}

function manageTimer(isRecording) {
  if (isRecording && state.tick === null) {
    state.tick = window.setInterval(async () => {
      render(await api().GetState());
    }, 1000);
  }
  if (!isRecording && state.tick !== null) {
    window.clearInterval(state.tick);
    state.tick = null;
  }
}

function managePreview(hasSource, index) {
  if (hasSource) {
    startNativePreview(index);
  }
  if (hasSource && !state.nativePreview && state.previewFrameRequest === null) {
    schedulePreviewFrame(index);
  }
  if (!hasSource && state.previewFrameRequest !== null) {
    window.cancelAnimationFrame(state.previewFrameRequest);
    state.previewFrameRequest = null;
    clearPreviewCanvas();
  }
  if (!hasSource && state.nativePreview) {
    api().StopNativePreview();
    state.nativePreview = false;
  }
}

async function startNativePreview(index) {
  if (index < 0) return;
  const selectedButton = [...els.sourceList.querySelectorAll(".source")][index];
  const isWindow = selectedButton?.querySelector(".thumb.window") || selectedButton?.querySelector(".thumb.layers") || selectedButton?.querySelector("[data-lucide='layers']");
  if (!isWindow) {
    if (state.nativePreview) {
      await api().StopNativePreview();
      state.nativePreview = false;
    }
    return;
  }

  const rect = els.previewCanvas.getBoundingClientRect();
  const x = Math.round(rect.left);
  const y = Math.round(rect.top);
  const width = Math.max(1, Math.round(rect.width));
  const height = Math.max(1, Math.round(rect.height));
  const started = await api().StartNativePreview(index, x, y, width, height);
  state.nativePreview = Boolean(started);
  if (state.nativePreview) {
    if (state.previewFrameRequest !== null) {
      window.cancelAnimationFrame(state.previewFrameRequest);
      state.previewFrameRequest = null;
    }
    clearPreviewCanvas();
  }
}

function moveNativePreview() {
  if (!state.nativePreview) return;
  const rect = els.previewCanvas.getBoundingClientRect();
  api().MoveNativePreview(
    Math.round(rect.left),
    Math.round(rect.top),
    Math.max(1, Math.round(rect.width)),
    Math.max(1, Math.round(rect.height)),
  );
}

function schedulePreviewFrame(index) {
  state.previewFrameRequest = window.requestAnimationFrame(() => updatePreview(index));
}

async function updatePreview(index) {
  state.previewFrameRequest = null;
  if (state.previewBusy || index < 0) return;
  state.previewBusy = true;
  try {
    const rect = els.previewCanvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const maxW = Math.max(1, Math.floor(rect.width * dpr));
    const maxH = Math.max(1, Math.floor(rect.height * dpr));
    const frame = await api().GetPreviewFrameRaw(index, maxW, maxH);
    if (frame?.pixels && index === state.selectedIndex) {
      drawPreviewFrame(frame, rect, dpr);
    }
  } finally {
    state.previewBusy = false;
    if (state.selectedIndex >= 0) {
      schedulePreviewFrame(state.selectedIndex);
    }
  }
}

function drawPreviewFrame(frame, rect, dpr) {
  const canvas = els.previewCanvas;
  const canvasWidth = Math.max(1, Math.floor(rect.width * dpr));
  const canvasHeight = Math.max(1, Math.floor(rect.height * dpr));
  if (canvas.width !== canvasWidth || canvas.height !== canvasHeight) {
    canvas.width = canvasWidth;
    canvas.height = canvasHeight;
    state.previewCtx = null;
  }

  const binary = window.atob(frame.pixels);
  const bytes = new Uint8ClampedArray(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }

  const ctx = state.previewCtx || canvas.getContext("2d", { alpha: false });
  state.previewCtx = ctx;
  const x = Math.max(0, Math.floor((canvas.width - frame.width) / 2));
  const y = Math.max(0, Math.floor((canvas.height - frame.height) / 2));
  ctx.fillStyle = "#090d12";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.putImageData(new ImageData(bytes, frame.width, frame.height), x, y);
  canvas.classList.add("live");
}

function clearPreviewCanvas() {
  const canvas = els.previewCanvas;
  const ctx = state.previewCtx || canvas.getContext("2d", { alpha: false });
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  canvas.classList.remove("live");
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function getNormalizedOverlayCoords() {
  const container = document.getElementById("previewCameraContainer");
  const previewArea = document.getElementById("previewArea");
  if (!container || !previewArea || container.classList.contains("hidden")) {
    return { x: 0, y: 0, w: 0, h: 0 };
  }
  const rect = container.getBoundingClientRect();
  const areaRect = previewArea.getBoundingClientRect();

  return {
    x: (rect.left - areaRect.left) / areaRect.width,
    y: (rect.top - areaRect.top) / areaRect.height,
    w: rect.width / areaRect.width,
    h: rect.height / areaRect.height,
  };
}

async function boot() {
  while (!api()) {
    await new Promise((resolve) => setTimeout(resolve, 20));
  }

  await loadOfflineAssets();
  await initMediaDevices();
  initOverlayDraggable();

  // Collapsible section event listeners
  const collapsibleConfigs = [
    { headerId: "videoHeader", sectionId: "videoSection" },
    { headerId: "camerasHeader", sectionId: "camerasSection" },
    { headerId: "audioHeader", sectionId: "audioSection" }
  ];

  collapsibleConfigs.forEach(({ headerId, sectionId }) => {
    const header = document.getElementById(headerId);
    const section = document.getElementById(sectionId);
    if (header && section) {
      header.addEventListener("click", () => {
        section.classList.toggle("expanded");
        const icon = header.querySelector(".chevron-icon");
        if (icon) {
          const isExp = section.classList.contains("expanded");
          icon.setAttribute("data-lucide", isExp ? "chevron-down" : "chevron-right");
          refreshIcons();
        }
      });
    }
  });

  // Attach tab events (Screens and Windows)
  els.tabs.forEach((tab) => {
    tab.addEventListener("click", async () => {
      const tabIndex = Number(tab.dataset.tab);
      render(await api().RefreshSources(tabIndex));
    });
  });

  els.refresh.addEventListener("click", async () => {
    await initMediaDevices();
    render(await api().RefreshSources(state.currentTab));
  });

  els.folder.addEventListener("click", () => api().OpenRecordingsFolder());

  els.record.addEventListener("click", async () => {
    if (state.isRecording) {
      render(await api().StopRecording());
      return;
    }

    const videoType = state.currentTab === 0 ? "screen" : "window";
    
    // Check if camera overlay is enabled
    let videoName = "";
    if (devicesState.selectedCameraId && devicesState.selectedCameraId !== "None") {
      const selectedCam = devicesState.cameras.find(c => c.deviceId === devicesState.selectedCameraId);
      if (selectedCam) {
        videoName = selectedCam.label;
      }
    }

    // Get floating camera normalized coordinates
    const coords = getNormalizedOverlayCoords();

    render(await api().StartRecording(
      state.selectedIndex,
      videoType,
      videoName,
      devicesState.selectedMics,
      coords.x,
      coords.y,
      coords.w,
      coords.h
    ));
  });

  els.minimise.addEventListener("click", () => api().Minimise());
  els.maximise.addEventListener("click", () => api().ToggleMaximise());
  els.close.addEventListener("click", () => api().Close());

  render(await api().GetState());
  window.addEventListener("resize", moveNativePreview);
  state.windowSaveTick = window.setInterval(() => api().SaveWindowState(), 5000);
  window.addEventListener("beforeunload", () => api().SaveWindowState());
}

function initOverlayDraggable() {
  const container = document.getElementById("previewCameraContainer");
  const previewArea = document.getElementById("previewArea");
  if (!container || !previewArea) return;

  let isDragging = false;
  let isResizing = false;
  let currentHandle = null;
  let startX, startY, startLeft, startTop, startWidth, startHeight;

  container.addEventListener("mousedown", (e) => {
    if (e.target.classList.contains("resize-handle")) {
      isResizing = true;
      currentHandle = e.target;
    } else {
      isDragging = true;
    }

    startX = e.clientX;
    startY = e.clientY;
    
    const rect = container.getBoundingClientRect();
    const areaRect = previewArea.getBoundingClientRect();

    startLeft = rect.left - areaRect.left;
    startTop = rect.top - areaRect.top;
    startWidth = rect.width;
    startHeight = rect.height;

    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
  });

  function onMouseMove(e) {
    const dx = e.clientX - startX;
    const dy = e.clientY - startY;
    const areaRect = previewArea.getBoundingClientRect();

    if (isDragging) {
      let newLeft = startLeft + dx;
      let newTop = startTop + dy;

      newLeft = Math.max(0, Math.min(newLeft, areaRect.width - startWidth));
      newTop = Math.max(0, Math.min(newTop, areaRect.height - startHeight));

      container.style.left = `${newLeft}px`;
      container.style.top = `${newTop}px`;
      container.style.bottom = "auto";
      container.style.right = "auto";
    } else if (isResizing) {
      let newWidth = startWidth;
      let newHeight = startHeight;
      let newLeft = startLeft;
      let newTop = startTop;

      if (currentHandle.classList.contains("bottom-right")) {
        newWidth = Math.max(80, startWidth + dx);
        newHeight = Math.max(60, startHeight + dy);
      } else if (currentHandle.classList.contains("bottom-left")) {
        newWidth = Math.max(80, startWidth - dx);
        newHeight = Math.max(60, startHeight + dy);
        newLeft = startLeft + (startWidth - newWidth);
      } else if (currentHandle.classList.contains("top-right")) {
        newWidth = Math.max(80, startWidth + dx);
        newHeight = Math.max(60, startHeight - dy);
        newTop = startTop + (startHeight - newHeight);
      } else if (currentHandle.classList.contains("top-left")) {
        newWidth = Math.max(80, startWidth - dx);
        newHeight = Math.max(60, startHeight - dy);
        newLeft = startLeft + (startWidth - newWidth);
        newTop = startTop + (startHeight - newHeight);
      }

      if (newLeft < 0) {
        newWidth += newLeft;
        newLeft = 0;
      }
      if (newTop < 0) {
        newHeight += newTop;
        newTop = 0;
      }
      if (newLeft + newWidth > areaRect.width) {
        newWidth = areaRect.width - newLeft;
      }
      if (newTop + newHeight > areaRect.height) {
        newHeight = areaRect.height - newTop;
      }

      container.style.width = `${newWidth}px`;
      container.style.height = `${newHeight}px`;
      container.style.left = `${newLeft}px`;
      container.style.top = `${newTop}px`;
      container.style.bottom = "auto";
      container.style.right = "auto";
    }
  }

  function onMouseUp() {
    isDragging = false;
    isResizing = false;
    currentHandle = null;
    document.removeEventListener("mousemove", onMouseMove);
    document.removeEventListener("mouseup", onMouseUp);
  }
}

async function initMediaDevices() {
  try {
    await navigator.mediaDevices.getUserMedia({ audio: true, video: true });
  } catch (err) {
    console.warn("Permissions not granted or failed:", err);
  }

  try {
    const list = await navigator.mediaDevices.enumerateDevices();
    devicesState.cameras = list.filter((d) => d.kind === "videoinput");
    devicesState.microphones = list.filter((d) => d.kind === "audioinput");
    
    if (devicesState.cameras.length > 0 && !devicesState.selectedCameraId) {
      devicesState.selectedCameraId = "None"; // default overlay off
    }
    if (devicesState.selectedMics.length === 0) {
      devicesState.selectedMics = ["None"];
    }
  } catch (err) {
    console.error("Failed to list media devices:", err);
  }
}

async function loadOfflineAssets() {
  const loaderStatus = document.getElementById("loader-status");
  let status = await api().GetAssetDownloadStatus();
  while (status.isDownloading) {
    loaderStatus.textContent = "Downloading system font & icon library...";
    await new Promise((r) => setTimeout(r, 800));
    status = await api().GetAssetDownloadStatus();
  }

  if (status.error) {
    console.error("Asset download error:", status.error);
  }

  try {
    loaderStatus.textContent = "Caching Urbanist font...";
    const fontBase64 = await api().LoadCachedAsset("Urbanist-Regular.ttf");
    if (fontBase64) {
      const fontFace = new FontFace("Urbanist", `url(data:font/ttf;base64,${fontBase64})`);
      await fontFace.load();
      document.fonts.add(fontFace);
      document.body.style.fontFamily = "'Urbanist', sans-serif";
    }
  } catch (err) {
    console.error("Failed to load Urbanist font:", err);
  }

  try {
    loaderStatus.textContent = "Caching icon library...";
    const lucideBase64 = await api().LoadCachedAsset("lucide.min.js");
    if (lucideBase64) {
      const decodedJs = atob(lucideBase64);
      const script = document.createElement("script");
      script.text = decodedJs;
      document.head.appendChild(script);
    }
  } catch (err) {
    console.error("Failed to load Lucide script:", err);
  }

  document.getElementById("app-loader").classList.add("hidden");
  document.getElementById("app-root").classList.remove("hidden");
}

boot();
