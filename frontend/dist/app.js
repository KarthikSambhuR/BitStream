const api = () => window.go?.main?.App;

const state = {
  selectedIndex: -1,
  currentTab: 0,
  isRecording: false,
  tick: null,
  previewTick: null,
  previewBusy: false,
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
  previewImage: document.querySelector("#previewImage"),
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
  els.previewImage.classList.toggle("live", Boolean(selected));

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
    ? "Selected source can be captured through Windows Graphics Capture. GPU frame-pool preview/encoding is the next active path."
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
  if (hasSource && state.previewTick === null) {
    updatePreview(index);
    state.previewTick = window.setInterval(() => updatePreview(state.selectedIndex), 125);
  }
  if (!hasSource && state.previewTick !== null) {
    window.clearInterval(state.previewTick);
    state.previewTick = null;
    els.previewImage.removeAttribute("src");
  }
}

async function updatePreview(index) {
  if (state.previewBusy || index < 0) return;
  state.previewBusy = true;
  try {
    const rect = els.previewImage.getBoundingClientRect();
    const maxW = Math.max(360, Math.min(960, Math.floor(rect.width - 44)));
    const maxH = Math.max(202, Math.min(540, Math.floor(rect.height - 44)));
    const frame = await api().GetPreviewFrame(index, maxW, maxH);
    if (frame && index === state.selectedIndex) {
      els.previewImage.src = frame;
    }
  } finally {
    state.previewBusy = false;
  }
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
  state.windowSaveTick = window.setInterval(() => api().SaveWindowState(), 5000);
  window.addEventListener("beforeunload", () => api().SaveWindowState());
}

boot();
