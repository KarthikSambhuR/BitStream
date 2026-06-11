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
  previewCard: document.querySelector(".preview-card"),
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
  return type === "screen" ? "Display source" : "Window source";
}

function render(next) {
  state.selectedIndex = next.selectedIndex;
  state.currentTab = next.currentTab;
  state.isRecording = next.isRecording;

  els.tabs.forEach((tab) => {
    tab.classList.toggle("active", Number(tab.dataset.tab) === next.currentTab);
  });

  els.sourceList.replaceChildren(
    ...next.sources.map((source) => {
      const button = document.createElement("button");
      button.className = `source ${source.index === next.selectedIndex ? "active" : ""}`;
      button.innerHTML = `
        <div class="thumb ${source.type}"></div>
        <div>
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
  els.record.textContent = next.isRecording ? "Stop Recording" : "Start Recording";
  els.record.classList.toggle("stop", next.isRecording);

  els.targetMem.textContent = next.memoryTargetMb;
  els.ceilingMem.textContent = next.memoryCeilingMb;
  els.backend.textContent = next.captureBackend;
  els.backendDetail.textContent = next.wgcCanCapture
    ? "Selected source is WGC-compatible. Browser capture prompts are disabled; native WGC preview is the next renderer path."
    : next.backendDetail;

  manageTimer(next.isRecording);
  managePreview(Boolean(selected), next.selectedIndex);
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
  const isWindow = selectedButton?.querySelector(".thumb.window");
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
  ctx.fillStyle = "#05080c";
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

async function boot() {
  while (!api()) {
    await new Promise((resolve) => setTimeout(resolve, 20));
  }

  els.tabs.forEach((tab) => {
    tab.addEventListener("click", async () => {
      render(await api().RefreshSources(Number(tab.dataset.tab)));
    });
  });

  els.refresh.addEventListener("click", async () => {
    render(await api().RefreshSources(state.currentTab));
  });

  els.folder.addEventListener("click", () => api().OpenRecordingsFolder());

  els.record.addEventListener("click", async () => {
    if (state.isRecording) {
      render(await api().StopRecording());
      return;
    }
    render(await api().StartRecording(state.selectedIndex));
  });

  els.minimise.addEventListener("click", () => api().Minimise());
  els.maximise.addEventListener("click", () => api().ToggleMaximise());
  els.close.addEventListener("click", () => api().Close());

  render(await api().GetState());
  window.addEventListener("resize", moveNativePreview);
  state.windowSaveTick = window.setInterval(() => api().SaveWindowState(), 5000);
  window.addEventListener("beforeunload", () => api().SaveWindowState());
}

boot();
