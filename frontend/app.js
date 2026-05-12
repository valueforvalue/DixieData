(() => {
  const timers = new WeakMap();
  const debugSurfaceIDsEnabled = () => document.body?.getAttribute("data-debug-ui-ids") === "true";
  const imageViewerState = {
    baseScale: 1,
    zoom: 1,
    x: 0,
    y: 0,
    dragging: false,
    lastPointerX: 0,
    lastPointerY: 0,
    fileName: "",
    imageId: "",
    imageUrl: "",
  };
  const scratchpadState = {
    displayId: "",
    dragging: false,
    pinned: true,
    pointerOffsetX: 0,
    pointerOffsetY: 0,
  };

  function getMethod(el) {
    if (el.hasAttribute("hx-post")) {
      return "POST";
    }
    if (el.hasAttribute("hx-put")) {
      return "PUT";
    }
    if (el.hasAttribute("hx-delete")) {
      return "DELETE";
    }
    return "GET";
  }

  function getUrl(el, method) {
    return el.getAttribute(`hx-${method.toLowerCase()}`);
  }

  function getTarget(el) {
    const selector = el.getAttribute("hx-target");
    if (!selector || selector === "body") {
      return document.body;
    }
    return document.querySelector(selector);
  }

  function getSwap(el) {
    return el.getAttribute("hx-swap") || "innerHTML";
  }

  function parseDelay(trigger) {
    const match = trigger.match(/delay:(\d+)ms/);
    if (!match) {
      return 0;
    }
    return Number.parseInt(match[1], 10);
  }

  function formDataFromElement(el) {
    const data = new FormData();
    if (el instanceof HTMLFormElement) {
      return new FormData(el);
    }
    const form = el.closest("form");
    if (form instanceof HTMLFormElement) {
      return new FormData(form);
    }
    if (el.name) {
      data.append(el.name, el.value ?? "");
    }
    return data;
  }

  function toSearchParams(data) {
    const params = new URLSearchParams();
    for (const [key, value] of data.entries()) {
      if (value instanceof File) {
        continue;
      }
      params.append(key, String(value));
    }
    return params;
  }

  function hasFiles(data) {
    for (const value of data.values()) {
      if (value instanceof File && value.name) {
        return true;
      }
    }
    return false;
  }

  function nativeMultipartMethodInput(form) {
    return form.querySelector("input[data-native-method-override]");
  }

  function submitMultipartForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return false;
    }
    const method = getMethod(form);
    const url = getUrl(form, method);
    if (!url) {
      return false;
    }

    let overrideInput = nativeMultipartMethodInput(form);
    if (method !== "POST") {
      if (!(overrideInput instanceof HTMLInputElement)) {
        overrideInput = document.createElement("input");
        overrideInput.type = "hidden";
        overrideInput.name = "_method";
        overrideInput.setAttribute("data-native-method-override", "true");
        form.appendChild(overrideInput);
      }
      overrideInput.value = method;
    } else if (overrideInput instanceof HTMLInputElement) {
      overrideInput.remove();
    }

    form.method = "post";
    form.action = url;
    form.submit();
    return true;
  }

  function activateTab(trigger) {
    const group = trigger.getAttribute("data-tab-group");
    const targetId = trigger.getAttribute("data-tab-target");
    if (!group || !targetId) {
      return;
    }

    document.querySelectorAll(`[data-tab-group="${group}"]`).forEach((button) => {
      const active = button === trigger;
      button.classList.toggle("bg-[#22303d]", active);
      button.classList.toggle("text-[#f4ead0]", active);
      button.classList.toggle("border-[#8d7440]", active);
      button.classList.toggle("shadow-[0_0_18px_rgba(168,138,70,0.18)]", active);
      button.classList.toggle("bg-[rgba(246,241,228,0.92)]", !active);
      button.classList.toggle("text-[#22303d]", !active);
    });

    document.querySelectorAll(`[data-tab-panel="${group}"]`).forEach((panel) => {
      panel.classList.toggle("hidden", panel.getAttribute("data-tab-id") !== targetId);
    });
  }

  function initializeTabs() {
    const defaults = new Map();
    document.querySelectorAll("[data-tab-group][data-tab-target]").forEach((button) => {
      const group = button.getAttribute("data-tab-group");
      if (!defaults.has(group) || button.hasAttribute("data-tab-default")) {
        defaults.set(group, button);
      }
    });
    defaults.forEach((button) => activateTab(button));
  }

  function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
  }

  function ensureImageViewer() {
    let viewer = document.getElementById("image-viewer");
    if (viewer) {
      return viewer;
    }

    viewer = document.createElement("div");
    viewer.id = "image-viewer";
    viewer.className = "fixed inset-0 z-50 hidden items-center justify-center bg-black/70 p-6";
    viewer.innerHTML = `
      <div data-ui-id="overlay.image.viewer" class="relative flex max-h-full w-full max-w-6xl flex-col overflow-hidden rounded-[2rem] border border-slate-700 bg-slate-950 shadow-2xl">
        ${debugSurfaceIDsEnabled() ? '<div class="ui-debug-badge" aria-hidden="true">overlay.image.viewer</div>' : ""}
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800 px-5 py-4 text-slate-200">
          <div>
            <p data-image-caption class="text-sm font-semibold tracking-[0.14em] text-slate-100"></p>
            <p data-image-file class="mt-1 text-xs text-slate-400"></p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span data-image-zoom-label class="rounded-full border border-slate-700 px-3 py-1 text-xs text-slate-300">100%</span>
            <button type="button" data-image-rotate-ccw class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">Rotate CCW</button>
            <button type="button" data-image-rotate-cw class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">Rotate CW</button>
            <button type="button" data-image-zoom-out class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">-</button>
            <button type="button" data-image-zoom-in class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">+</button>
            <button type="button" data-image-reset class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">Reset</button>
            <button type="button" data-image-screenshot class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">Screenshot</button>
            <button type="button" data-image-close class="rounded-full border border-slate-700 px-3 py-1 text-sm text-slate-200">Close</button>
          </div>
        </div>
        <div data-image-stage class="relative h-[78vh] overflow-hidden bg-slate-900">
          <img data-image-element class="absolute left-1/2 top-1/2 max-w-none select-none rounded-2xl bg-white shadow-2xl" alt="" draggable="false" />
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3 border-t border-slate-800 px-5 py-3 text-xs text-slate-400">
          <p>Mouse wheel zooms. Drag to move when zoomed in.</p>
          <p data-image-status></p>
        </div>
      </div>
    `;
    document.body.appendChild(viewer);
    return viewer;
  }

  function imageViewerElements() {
    const viewer = ensureImageViewer();
    return {
      viewer,
      stage: viewer.querySelector("[data-image-stage]"),
      image: viewer.querySelector("[data-image-element]"),
      caption: viewer.querySelector("[data-image-caption]"),
      file: viewer.querySelector("[data-image-file]"),
      zoomLabel: viewer.querySelector("[data-image-zoom-label]"),
      status: viewer.querySelector("[data-image-status]"),
    };
  }

  function setImageViewerStatus(message) {
    const { status } = imageViewerElements();
    if (status) {
      status.textContent = message || "";
    }
  }

  function stopImageViewerDrag() {
    imageViewerState.dragging = false;
    updateImageViewerTransform();
  }

  function updateImageViewerTransform() {
    const { stage, image, zoomLabel } = imageViewerElements();
    if (!(stage instanceof HTMLElement) || !(image instanceof HTMLImageElement) || !image.naturalWidth || !image.naturalHeight) {
      return;
    }

    const stageRect = stage.getBoundingClientRect();
    if (!stageRect.width || !stageRect.height) {
      return;
    }

    const totalScale = imageViewerState.baseScale * imageViewerState.zoom;
    const scaledWidth = image.naturalWidth * totalScale;
    const scaledHeight = image.naturalHeight * totalScale;
    const maxX = Math.max(0, (scaledWidth - stageRect.width) / 2);
    const maxY = Math.max(0, (scaledHeight - stageRect.height) / 2);

    imageViewerState.x = clamp(imageViewerState.x, -maxX, maxX);
    imageViewerState.y = clamp(imageViewerState.y, -maxY, maxY);

    image.style.transform = `translate(-50%, -50%) translate(${imageViewerState.x}px, ${imageViewerState.y}px) scale(${totalScale})`;
    image.style.cursor = imageViewerState.zoom > 1 ? (imageViewerState.dragging ? "grabbing" : "grab") : "default";

    if (zoomLabel) {
      zoomLabel.textContent = `${Math.round(imageViewerState.zoom * 100)}%`;
    }
  }

  function resetImageViewerTransform() {
    const { stage, image } = imageViewerElements();
    if (!(stage instanceof HTMLElement) || !(image instanceof HTMLImageElement) || !image.naturalWidth || !image.naturalHeight) {
      return;
    }

    window.requestAnimationFrame(() => {
      const stageRect = stage.getBoundingClientRect();
      if (!stageRect.width || !stageRect.height) {
        return;
      }

      const widthScale = stageRect.width / image.naturalWidth;
      const heightScale = stageRect.height / image.naturalHeight;
      imageViewerState.baseScale = Math.min(widthScale, heightScale);
      if (!Number.isFinite(imageViewerState.baseScale) || imageViewerState.baseScale <= 0) {
        imageViewerState.baseScale = 1;
      }
      imageViewerState.zoom = 1;
      imageViewerState.x = 0;
      imageViewerState.y = 0;
      updateImageViewerTransform();
    });
  }

  function setImageViewerZoom(nextZoom, pointerX = 0, pointerY = 0) {
    const { image } = imageViewerElements();
    if (!(image instanceof HTMLImageElement) || !image.naturalWidth || !image.naturalHeight) {
      return;
    }

    const oldTotalScale = imageViewerState.baseScale * imageViewerState.zoom;
    imageViewerState.zoom = clamp(nextZoom, 1, 6);
    const newTotalScale = imageViewerState.baseScale * imageViewerState.zoom;

    if (oldTotalScale > 0 && newTotalScale > 0) {
      imageViewerState.x = pointerX - ((pointerX - imageViewerState.x) / oldTotalScale) * newTotalScale;
      imageViewerState.y = pointerY - ((pointerY - imageViewerState.y) / oldTotalScale) * newTotalScale;
    }

    updateImageViewerTransform();
  }

  async function saveImageViewerScreenshot() {
    const { stage, image } = imageViewerElements();
    if (!(stage instanceof HTMLElement) || !(image instanceof HTMLImageElement) || !image.complete || !image.naturalWidth || !image.naturalHeight) {
      return;
    }

    const width = Math.max(1, Math.round(stage.clientWidth));
    const height = Math.max(1, Math.round(stage.clientHeight));
    const dpr = window.devicePixelRatio || 1;
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(width * dpr));
    canvas.height = Math.max(1, Math.round(height * dpr));

    const context = canvas.getContext("2d");
    if (!context) {
      setImageViewerStatus("Screenshot failed.");
      return;
    }

    context.scale(dpr, dpr);
    context.fillStyle = "#0f172a";
    context.fillRect(0, 0, width, height);

    const totalScale = imageViewerState.baseScale * imageViewerState.zoom;
    const drawWidth = image.naturalWidth * totalScale;
    const drawHeight = image.naturalHeight * totalScale;
    const drawX = width / 2 - drawWidth / 2 + imageViewerState.x;
    const drawY = height / 2 - drawHeight / 2 + imageViewerState.y;

    context.drawImage(image, drawX, drawY, drawWidth, drawHeight);
    setImageViewerStatus("Saving screenshot...");

    try {
      const response = await fetch("/images/screenshot", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Requested-With": "DixieData",
        },
        body: JSON.stringify({
          imageData: canvas.toDataURL("image/png"),
          fileName: imageViewerState.fileName,
        }),
      });
      const text = await response.text();
      setImageViewerStatus(text);
    } catch (error) {
      setImageViewerStatus("Screenshot failed.");
    }
  }

  function cacheBustedImageURL(url) {
    if (!url) {
      return url;
    }
    const separator = url.includes("?") ? "&" : "?";
    return `${url}${separator}v=${Date.now()}`;
  }

  function refreshImageReferences(imageId, baseUrl) {
    if (!imageId || !baseUrl) {
      return;
    }
    const refreshedUrl = cacheBustedImageURL(baseUrl);
    document.querySelectorAll(`[data-image-thumb-id="${imageId}"]`).forEach((thumb) => {
      if (thumb instanceof HTMLImageElement) {
        thumb.src = refreshedUrl;
      }
    });
    document.querySelectorAll(`[data-image-id="${imageId}"]`).forEach((button) => {
      if (button instanceof HTMLElement) {
        button.setAttribute("data-image-preview", refreshedUrl);
      }
    });
    imageViewerState.imageUrl = baseUrl;
    return refreshedUrl;
  }

  async function rotateImageViewer(direction) {
    if (!imageViewerState.imageId) {
      setImageViewerStatus("Image rotate failed.");
      return;
    }
    setImageViewerStatus(`Rotating image ${direction.toUpperCase()}...`);
    try {
      const response = await fetch("/images/rotate", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Requested-With": "DixieData",
        },
        body: JSON.stringify({
          imageId: Number.parseInt(imageViewerState.imageId, 10),
          direction,
        }),
      });
      const text = await response.text();
      if (!response.ok) {
        setImageViewerStatus(text || "Image rotate failed.");
        return;
      }
      const refreshedUrl = refreshImageReferences(imageViewerState.imageId, imageViewerState.imageUrl);
      if (refreshedUrl) {
        const { image } = imageViewerElements();
        if (image instanceof HTMLImageElement) {
          image.onload = () => {
            setImageViewerStatus(text || "Image rotated.");
            resetImageViewerTransform();
          };
          image.src = refreshedUrl;
        }
      } else {
        setImageViewerStatus(text || "Image rotated.");
      }
    } catch (error) {
      setImageViewerStatus("Image rotate failed.");
    }
  }

  function openImageViewer(url, caption, fileName, imageId) {
    const { viewer, image, caption: text, file } = imageViewerElements();
    if (!(image instanceof HTMLImageElement) || !text || !file) {
      return;
    }

    imageViewerState.fileName = fileName || "";
    imageViewerState.imageId = imageId || "";
    imageViewerState.imageUrl = url || "";
    image.onload = () => {
      setImageViewerStatus("");
      resetImageViewerTransform();
    };
    image.onerror = () => {
      setImageViewerStatus("Image preview failed. The stored file may be empty or invalid.");
    };
    image.setAttribute("src", url);
    image.setAttribute("alt", caption || fileName || "Archive image");
    text.textContent = caption || fileName || "Archive image";
    file.textContent = fileName || "";
    setImageViewerStatus("");
    viewer.classList.remove("hidden");
    viewer.classList.add("flex");

    if (image.complete) {
      resetImageViewerTransform();
    }
  }

  function closeImageViewer() {
    const { viewer } = imageViewerElements();
    if (!viewer) {
      return;
    }
    imageViewerState.dragging = false;
    viewer.classList.add("hidden");
    viewer.classList.remove("flex");
  }

  function scratchpadDisplayId(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    const field = form.querySelector('input[name="display_id"]');
    if (!(field instanceof HTMLInputElement)) {
      return "";
    }
    return (field.value || "").trim();
  }

  function scratchpadFormFromElement(el) {
    if (el instanceof HTMLFormElement) {
      return el;
    }
    if (el instanceof HTMLElement) {
      const form = el.closest("form");
      if (form instanceof HTMLFormElement) {
        return form;
      }
    }
    const page = document.querySelector('[data-ui-id="page.soldier.new"], [data-ui-id="page.soldier.edit"]');
    if (page instanceof HTMLElement) {
      const form = page.querySelector("form");
      if (form instanceof HTMLFormElement) {
        return form;
      }
    }
    return null;
  }

  function normalizeScratchpadDisplayId(displayId) {
    const value = (displayId || "").trim();
    return value || "unfiled";
  }

  function scratchpadContentKey(displayId) {
    return `dixiedata:scratchpad:${normalizeScratchpadDisplayId(displayId)}`;
  }

  function scratchpadWindowStateKey(displayId) {
    return `dixiedata:scratchpad-window:${normalizeScratchpadDisplayId(displayId)}`;
  }

  function loadScratchpadText(displayId) {
    try {
      return window.localStorage.getItem(scratchpadContentKey(displayId)) || "";
    } catch (error) {
      return "";
    }
  }

  function saveScratchpadText(displayId, value) {
    try {
      window.localStorage.setItem(scratchpadContentKey(displayId), value || "");
    } catch (error) {
    }
  }

  function loadScratchpadWindowState(displayId) {
    try {
      const raw = window.localStorage.getItem(scratchpadWindowStateKey(displayId));
      if (!raw) {
        return null;
      }
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object") {
        return null;
      }
      return parsed;
    } catch (error) {
      return null;
    }
  }

  function saveScratchpadWindowState(displayId, state) {
    try {
      window.localStorage.setItem(scratchpadWindowStateKey(displayId), JSON.stringify(state));
    } catch (error) {
    }
  }

  function ensureScratchpadWindow() {
    let panel = document.getElementById("scratchpad-window");
    if (panel) {
      return panel;
    }

    panel = document.createElement("div");
    panel.id = "scratchpad-window";
    panel.setAttribute("data-ui-id", "overlay.soldier.scratchpad");
    panel.className = "fixed hidden flex-col rounded-[1.7rem] border border-[rgba(141,116,64,0.72)] bg-[rgba(246,241,228,0.98)] shadow-[0_28px_80px_rgba(23,33,43,0.28)]";
    panel.style.top = "96px";
    panel.style.left = "96px";
    panel.style.width = "440px";
    panel.style.height = "320px";
    panel.style.resize = "both";
    panel.style.overflow = "hidden";
    panel.style.minWidth = "320px";
    panel.style.minHeight = "220px";
    panel.innerHTML = `
      ${debugSurfaceIDsEnabled() ? '<div class="ui-debug-badge" aria-hidden="true">overlay.soldier.scratchpad</div>' : ""}
      <div data-scratchpad-drag-handle class="flex cursor-move items-center justify-between gap-3 border-b border-[rgba(141,116,64,0.35)] bg-[rgba(36,48,61,0.08)] px-4 py-3">
        <div>
          <p data-scratchpad-title class="text-sm font-semibold uppercase tracking-[0.18em] text-[#22303d]">Scratch Pad</p>
          <p data-scratchpad-subtitle class="mt-1 text-xs text-slate-500"></p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" data-scratchpad-copy class="pill-link">Copy All</button>
          <button type="button" data-scratchpad-clear class="pill-link">Clear</button>
          <button type="button" data-scratchpad-pin class="pill-link">Pinned</button>
          <button type="button" data-scratchpad-close class="pill-link">Close</button>
        </div>
      </div>
      <div class="flex-1 p-3">
        <textarea data-scratchpad-text class="field-input h-full min-h-0 resize-none" placeholder="Paste temporary notes here..."></textarea>
      </div>
      <div class="flex items-center justify-between border-t border-[rgba(141,116,64,0.28)] px-4 py-2 text-xs text-slate-500">
        <span>Local only. Stored per Record ID outside the database.</span>
        <span>Drag the header and resize from the bottom-right corner.</span>
      </div>
    `;
    document.body.appendChild(panel);

    if (typeof ResizeObserver === "function") {
      const resizeObserver = new ResizeObserver(() => {
        persistScratchpadWindowState();
      });
      resizeObserver.observe(panel);
    }

    return panel;
  }

  function scratchpadElements() {
    const panel = ensureScratchpadWindow();
    return {
      panel,
      title: panel.querySelector("[data-scratchpad-title]"),
      subtitle: panel.querySelector("[data-scratchpad-subtitle]"),
      textarea: panel.querySelector("[data-scratchpad-text]"),
      pinButton: panel.querySelector("[data-scratchpad-pin]"),
    };
  }

  function applyScratchpadPinnedState() {
    const { panel, pinButton } = scratchpadElements();
    panel.style.zIndex = scratchpadState.pinned ? "80" : "45";
    if (pinButton instanceof HTMLElement) {
      pinButton.textContent = scratchpadState.pinned ? "Pinned" : "Pin";
      pinButton.classList.toggle("primary-button", scratchpadState.pinned);
      pinButton.classList.toggle("pill-link", !scratchpadState.pinned);
    }
  }

  function scratchpadWindowStateFromPanel() {
    const { panel } = scratchpadElements();
    const rect = panel.getBoundingClientRect();
    return {
      left: Math.round(rect.left),
      top: Math.round(rect.top),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
      pinned: !!scratchpadState.pinned,
    };
  }

  function persistScratchpadWindowState() {
    if (!scratchpadState.displayId) {
      return;
    }
    const { panel } = scratchpadElements();
    if (panel.classList.contains("hidden")) {
      return;
    }
    saveScratchpadWindowState(scratchpadState.displayId, scratchpadWindowStateFromPanel());
  }

  function applyScratchpadWindowState(displayId) {
    const { panel } = scratchpadElements();
    const saved = loadScratchpadWindowState(displayId) || {};
    panel.style.left = `${Number.isFinite(saved.left) ? saved.left : 96}px`;
    panel.style.top = `${Number.isFinite(saved.top) ? saved.top : 96}px`;
    panel.style.width = `${Number.isFinite(saved.width) ? saved.width : 440}px`;
    panel.style.height = `${Number.isFinite(saved.height) ? saved.height : 320}px`;
    scratchpadState.pinned = saved.pinned !== false;
    applyScratchpadPinnedState();
  }

  function refreshScratchpadContext(form) {
    const nextDisplayId = scratchpadDisplayId(form);
    if (!nextDisplayId || nextDisplayId === scratchpadState.displayId) {
      return;
    }
    const { textarea, title, subtitle } = scratchpadElements();
    if (!(textarea instanceof HTMLTextAreaElement)) {
      return;
    }
    if (scratchpadState.displayId) {
      saveScratchpadText(scratchpadState.displayId, textarea.value);
      persistScratchpadWindowState();
    }
    scratchpadState.displayId = nextDisplayId;
    textarea.value = loadScratchpadText(nextDisplayId);
    if (title) {
      title.textContent = `Scratch Pad`;
    }
    if (subtitle) {
      subtitle.textContent = `Record ID: ${nextDisplayId}`;
    }
    applyScratchpadWindowState(nextDisplayId);
  }

  function openScratchpad(trigger) {
    const form = scratchpadFormFromElement(trigger);
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const displayId = scratchpadDisplayId(form);
    if (!displayId) {
      window.alert("A Record ID is required before opening the scratch pad.");
      return;
    }
    const { panel, textarea, title, subtitle } = scratchpadElements();
    if (!(textarea instanceof HTMLTextAreaElement)) {
      return;
    }
    scratchpadState.displayId = displayId;
    applyScratchpadWindowState(displayId);
    textarea.value = loadScratchpadText(displayId);
    if (title) {
      title.textContent = "Scratch Pad";
    }
    if (subtitle) {
      subtitle.textContent = `Record ID: ${displayId}`;
    }
    panel.classList.remove("hidden");
    panel.classList.add("flex");
    applyScratchpadPinnedState();
    textarea.focus();
  }

  function closeScratchpad() {
    const { panel, textarea } = scratchpadElements();
    if (textarea instanceof HTMLTextAreaElement && scratchpadState.displayId) {
      saveScratchpadText(scratchpadState.displayId, textarea.value);
      persistScratchpadWindowState();
    }
    scratchpadState.dragging = false;
    panel.classList.add("hidden");
    panel.classList.remove("flex");
  }

  async function copyScratchpadText() {
    const { textarea } = scratchpadElements();
    if (!(textarea instanceof HTMLTextAreaElement)) {
      return;
    }
    const value = textarea.value || "";
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(value);
        return;
      }
    } catch (error) {
    }
    textarea.focus();
    textarea.select();
    document.execCommand("copy");
  }

  function clearScratchpadText() {
    const { textarea } = scratchpadElements();
    if (!(textarea instanceof HTMLTextAreaElement)) {
      return;
    }
    textarea.value = "";
    if (scratchpadState.displayId) {
      saveScratchpadText(scratchpadState.displayId, "");
    }
    textarea.focus();
  }

  function toggleCheckboxGroup(group, checked) {
    document.querySelectorAll(`[data-checkbox-group="${group}"]`).forEach((checkbox) => {
      if (checkbox instanceof HTMLInputElement) {
        checkbox.checked = checked;
      }
    });
  }

  function addRecordRow(button) {
    const container = button.closest("form");
    if (!(container instanceof HTMLFormElement)) {
      return;
    }
    const recordList = container.querySelector("[data-record-list]");
    const template = container.querySelector("[data-record-template]");
    if (!(recordList instanceof HTMLElement) || !(template instanceof HTMLTemplateElement)) {
      return;
    }
    recordList.appendChild(template.content.cloneNode(true));
  }

  function removeRecordRow(button) {
    const row = button.closest("[data-record-row]");
    if (!(row instanceof HTMLElement)) {
      return;
    }
    const recordList = row.parentElement;
    if (!(recordList instanceof HTMLElement)) {
      return;
    }
    const rows = recordList.querySelectorAll("[data-record-row]");
    if (rows.length <= 1) {
      row.querySelectorAll("input, textarea").forEach((field) => {
        if (field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement) {
          field.value = "";
        }
      });
      return;
    }
    row.remove();
  }

  function draftKeyForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    return form.getAttribute("data-draft-key") || "";
  }

  function persistDraftForForm(form) {
    const key = draftKeyForForm(form);
    if (!key) {
      return;
    }
    const payload = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement || field instanceof HTMLSelectElement)) {
        return;
      }
      if (!field.name || field.disabled || field instanceof HTMLInputElement && field.type === "file") {
        return;
      }
      if (!Object.prototype.hasOwnProperty.call(payload, field.name)) {
        payload[field.name] = [];
      }
      payload[field.name].push(field.value ?? "");
    });
    try {
      window.localStorage.setItem(`dixiedata:${key}`, JSON.stringify(payload));
    } catch (error) {
    }
  }

  function clearDraftForForm(form) {
    const key = draftKeyForForm(form);
    if (!key) {
      return;
    }
    try {
      window.localStorage.removeItem(`dixiedata:${key}`);
    } catch (error) {
    }
  }

  function ensureRecordRowCount(form, targetCount) {
    if (!(form instanceof HTMLFormElement) || targetCount < 1) {
      return;
    }
    let rows = form.querySelectorAll("[data-record-row]");
    while (rows.length < targetCount) {
      const addButton = form.querySelector("[data-record-add]");
      if (!(addButton instanceof HTMLElement)) {
        break;
      }
      addRecordRow(addButton);
      rows = form.querySelectorAll("[data-record-row]");
    }
  }

  function restoreDraftForForm(form) {
    const key = draftKeyForForm(form);
    if (!key) {
      return;
    }
    let saved;
    try {
      saved = window.localStorage.getItem(`dixiedata:${key}`);
    } catch (error) {
      return;
    }
    if (!saved) {
      return;
    }
    let payload;
    try {
      payload = JSON.parse(saved);
    } catch (error) {
      return;
    }
    const recordCount = Math.max(
      1,
      Array.isArray(payload.record_type) ? payload.record_type.length : 0,
      Array.isArray(payload.record_app_id) ? payload.record_app_id.length : 0,
      Array.isArray(payload.record_details) ? payload.record_details.length : 0,
    );
    ensureRecordRowCount(form, recordCount);
    const cursors = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement || field instanceof HTMLSelectElement)) {
        return;
      }
      if (!field.name || field.disabled || field instanceof HTMLInputElement && field.type === "file") {
        return;
      }
      const values = payload[field.name];
      if (!Array.isArray(values) || values.length === 0) {
        return;
      }
      const index = cursors[field.name] || 0;
      if (index >= values.length) {
        return;
      }
      field.value = values[index] ?? "";
      cursors[field.name] = index + 1;
    });
  }

  function initializeDraftForms() {
    document.querySelectorAll("form[data-draft-key]").forEach((form) => {
      if (form instanceof HTMLFormElement) {
        restoreDraftForForm(form);
      }
    });
  }

  function renderDocument(html) {
    document.open();
    document.write(html);
    document.close();
  }

  function applyResponse(el, html) {
    const target = getTarget(el);
    if (!target) {
      return;
    }

    const trimmed = html.trimStart().toLowerCase();
    if (target === document.body && trimmed.startsWith("<!doctype html")) {
      renderDocument(html);
      return;
    }

    if (target === document.body && trimmed.startsWith("<html")) {
      renderDocument(html);
      return;
    }

    if (getSwap(el) === "outerHTML") {
      target.outerHTML = html;
      return;
    }

    target.innerHTML = html;
  }

  function showProgress(el) {
    const target = getTarget(el);
    if (!(target instanceof HTMLElement) || target === document.body) {
      return;
    }
    const label = el.getAttribute("data-progress-label") || "Working...";
    target.innerHTML = `
      <div class="google-progress-shell">
        <div class="google-progress-head">
          <span>${label}</span>
          <span>Please wait...</span>
        </div>
        <div class="google-progress-track">
          <div class="google-progress-fill"></div>
        </div>
      </div>
    `;
  }

  function setBusyState(el, busy) {
    if (!(el instanceof HTMLElement)) {
      return;
    }
    if (busy) {
      el.setAttribute("aria-busy", "true");
      if (el instanceof HTMLButtonElement) {
        el.disabled = true;
      }
      return;
    }
    el.removeAttribute("aria-busy");
    if (el instanceof HTMLButtonElement) {
      el.disabled = false;
    }
  }

  async function request(el) {
    const method = getMethod(el);
    const url = getUrl(el, method);
    if (!url) {
      return;
    }

    const confirmMessage = el.getAttribute("hx-confirm");
    if (confirmMessage && !window.confirm(confirmMessage)) {
      return;
    }

    const data = formDataFromElement(el);
    const includesFiles = hasFiles(data);
    if (includesFiles && el instanceof HTMLFormElement && submitMultipartForm(el)) {
      return;
    }
    const transportMethod = includesFiles && method !== "GET" && method !== "POST" ? "POST" : method;
    const options = {
      method: transportMethod,
      headers: {
        "X-Requested-With": "DixieData"
      }
    };
    if (transportMethod !== method) {
      options.headers["X-HTTP-Method-Override"] = method;
    }

    let requestUrl = url;
    if (method === "GET") {
      const params = toSearchParams(data);
      const query = params.toString();
      if (query) {
        requestUrl += (url.includes("?") ? "&" : "?") + query;
      }
    } else {
      if (includesFiles) {
        options.body = data;
      } else {
        options.headers["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8";
        options.body = toSearchParams(data).toString();
      }
    }

    showProgress(el);
    setBusyState(el, true);
    try {
      const response = await fetch(requestUrl, options);
      const html = await response.text();
      const redirectTo = response.headers.get("X-DixieData-Redirect");
      if (redirectTo) {
        window.location.assign(redirectTo);
        return;
      }
      if (el instanceof HTMLFormElement && response.ok && response.redirected) {
        clearDraftForForm(el);
      }
      applyResponse(el, html);
    } catch (error) {
      applyResponse(el, "Request failed.");
    } finally {
      setBusyState(el, false);
    }
  }

  function queueRequest(el) {
    const trigger = el.getAttribute("hx-trigger") || "";
    const delay = parseDelay(trigger);
    const existingTimer = timers.get(el);
    if (existingTimer) {
      window.clearTimeout(existingTimer);
    }
    const timer = window.setTimeout(() => {
      timers.delete(el);
      request(el);
    }, delay);
    timers.set(el, timer);
  }

  async function openExternalLinkInChrome(href) {
    const params = new URLSearchParams();
    params.set("target", href);
    try {
      const response = await fetch("/open-link", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
        },
        body: params.toString(),
      });
      if (!response.ok) {
        console.error(await response.text());
      }
    } catch (error) {
      console.error("Failed to open external link", error);
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    initializeTabs();
    initializeDraftForms();
    document.querySelectorAll('[hx-trigger="load"]').forEach((el) => {
      request(el);
    });
  });

  document.addEventListener("click", (event) => {
    const externalLink = event.target.closest("a[data-open-external]");
    if (externalLink instanceof HTMLAnchorElement) {
      event.preventDefault();
      openExternalLinkInChrome(externalLink.href);
      return;
    }
    const imageTrigger = event.target.closest("[data-image-preview]");
    if (imageTrigger) {
      event.preventDefault();
      openImageViewer(
        imageTrigger.getAttribute("data-image-preview"),
        imageTrigger.getAttribute("data-image-caption"),
        imageTrigger.getAttribute("data-image-file"),
        imageTrigger.getAttribute("data-image-id"),
      );
      return;
    }
    const scratchpadOpen = event.target.closest("[data-scratchpad-open]");
    if (scratchpadOpen) {
      event.preventDefault();
      openScratchpad(scratchpadOpen);
      return;
    }
    if (event.target.closest("[data-scratchpad-copy]")) {
      event.preventDefault();
      copyScratchpadText();
      return;
    }
    if (event.target.closest("[data-scratchpad-clear]")) {
      event.preventDefault();
      clearScratchpadText();
      return;
    }
    if (event.target.closest("[data-scratchpad-pin]")) {
      event.preventDefault();
      scratchpadState.pinned = !scratchpadState.pinned;
      applyScratchpadPinnedState();
      persistScratchpadWindowState();
      return;
    }
    if (event.target.closest("[data-scratchpad-close]")) {
      event.preventDefault();
      closeScratchpad();
      return;
    }
    if (event.target.closest("[data-image-rotate-ccw]")) {
      event.preventDefault();
      rotateImageViewer("ccw");
      return;
    }
    if (event.target.closest("[data-image-rotate-cw]")) {
      event.preventDefault();
      rotateImageViewer("cw");
      return;
    }
    if (event.target.closest("[data-image-zoom-in]")) {
      event.preventDefault();
      setImageViewerZoom(imageViewerState.zoom * 1.2);
      return;
    }
    if (event.target.closest("[data-image-zoom-out]")) {
      event.preventDefault();
      setImageViewerZoom(imageViewerState.zoom / 1.2);
      return;
    }
    if (event.target.closest("[data-image-reset]")) {
      event.preventDefault();
      resetImageViewerTransform();
      return;
    }
    if (event.target.closest("[data-image-screenshot]")) {
      event.preventDefault();
      saveImageViewerScreenshot();
      return;
    }
    const recordAdd = event.target.closest("[data-record-add]");
    if (recordAdd) {
      event.preventDefault();
      addRecordRow(recordAdd);
      const form = recordAdd.closest("form");
      if (form instanceof HTMLFormElement) {
        persistDraftForForm(form);
      }
      return;
    }
    const recordRemove = event.target.closest("[data-record-remove]");
    if (recordRemove) {
      event.preventDefault();
      removeRecordRow(recordRemove);
      const form = recordRemove.closest("form");
      if (form instanceof HTMLFormElement) {
        persistDraftForForm(form);
      }
      return;
    }
    const imageClose = event.target.closest("[data-image-close]");
    if (imageClose || event.target.id === "image-viewer") {
      event.preventDefault();
      closeImageViewer();
      return;
    }
    const tab = event.target.closest("[data-tab-group][data-tab-target]");
    if (tab) {
      event.preventDefault();
      activateTab(tab);
      return;
    }
    const el = event.target.closest("[hx-get],[hx-post],[hx-delete]");
    if (!el || el instanceof HTMLFormElement) {
      return;
    }
    event.preventDefault();
    request(el);
  });

  document.addEventListener("submit", (event) => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    if (!form.hasAttribute("hx-get") && !form.hasAttribute("hx-post") && !form.hasAttribute("hx-put")) {
      return;
    }
    event.preventDefault();
    request(form);
  });

  const triggerInputRequest = (event) => {
    const el = event.target.closest("[hx-trigger]");
    if (!el) {
      return;
    }
    const trigger = el.getAttribute("hx-trigger") || "";
    if (!trigger.includes("keyup") && !trigger.includes("changed")) {
      return;
    }
    queueRequest(el);
  };

  document.addEventListener("input", triggerInputRequest);
  document.addEventListener("change", triggerInputRequest);
  document.addEventListener("input", (event) => {
    const field = event.target;
    if (field instanceof HTMLInputElement && field.name === "display_id") {
      const form = field.closest("form");
      refreshScratchpadContext(form);
      return;
    }
    if (field instanceof HTMLTextAreaElement && field.hasAttribute("data-scratchpad-text") && scratchpadState.displayId) {
      saveScratchpadText(scratchpadState.displayId, field.value);
    }
  });
  document.addEventListener("input", (event) => {
    const form = event.target.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      persistDraftForForm(form);
    }
  });
  document.addEventListener("change", (event) => {
    const field = event.target;
    if (field instanceof HTMLInputElement && field.name === "display_id") {
      const form = field.closest("form");
      refreshScratchpadContext(form);
      return;
    }
    const form = event.target.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      persistDraftForForm(form);
    }
  });
  document.addEventListener("change", (event) => {
    const selectAll = event.target.closest("[data-select-all]");
    if (!(selectAll instanceof HTMLInputElement)) {
      return;
    }
    toggleCheckboxGroup(selectAll.getAttribute("data-select-all"), selectAll.checked);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeImageViewer();
    }
  });
  document.addEventListener("mousedown", (event) => {
    const scratchpadHandle = event.target.closest("[data-scratchpad-drag-handle]");
    if (scratchpadHandle instanceof HTMLElement) {
      const { panel } = scratchpadElements();
      if (!panel.classList.contains("hidden")) {
        const rect = panel.getBoundingClientRect();
        scratchpadState.dragging = true;
        scratchpadState.pointerOffsetX = event.clientX - rect.left;
        scratchpadState.pointerOffsetY = event.clientY - rect.top;
        event.preventDefault();
        return;
      }
    }
    const stage = event.target.closest("[data-image-stage]");
    if (!(stage instanceof HTMLElement) || imageViewerState.zoom <= 1) {
      return;
    }
    event.preventDefault();
    imageViewerState.dragging = true;
    imageViewerState.lastPointerX = event.clientX;
    imageViewerState.lastPointerY = event.clientY;
    updateImageViewerTransform();
  });
  document.addEventListener("mousemove", (event) => {
    if (scratchpadState.dragging) {
      const { panel } = scratchpadElements();
      panel.style.left = `${event.clientX - scratchpadState.pointerOffsetX}px`;
      panel.style.top = `${event.clientY - scratchpadState.pointerOffsetY}px`;
      return;
    }
    if (!imageViewerState.dragging) {
      return;
    }
    imageViewerState.x += event.clientX - imageViewerState.lastPointerX;
    imageViewerState.y += event.clientY - imageViewerState.lastPointerY;
    imageViewerState.lastPointerX = event.clientX;
    imageViewerState.lastPointerY = event.clientY;
    updateImageViewerTransform();
  });
  document.addEventListener("mouseup", () => {
    if (scratchpadState.dragging) {
      scratchpadState.dragging = false;
      persistScratchpadWindowState();
    }
    stopImageViewerDrag();
  });
  document.addEventListener("mouseleave", () => {
    scratchpadState.dragging = false;
    stopImageViewerDrag();
  });
  window.addEventListener("blur", () => {
    scratchpadState.dragging = false;
    stopImageViewerDrag();
  });
  window.addEventListener("resize", () => {
    const viewer = document.getElementById("image-viewer");
    if (viewer && !viewer.classList.contains("hidden")) {
      resetImageViewerTransform();
    }
    persistScratchpadWindowState();
  });
  document.addEventListener(
    "wheel",
    (event) => {
      const stage = event.target.closest("[data-image-stage]");
      if (!(stage instanceof HTMLElement)) {
        return;
      }
      event.preventDefault();
      const rect = stage.getBoundingClientRect();
      const pointerX = event.clientX - rect.left - rect.width / 2;
      const pointerY = event.clientY - rect.top - rect.height / 2;
      const zoomFactor = event.deltaY < 0 ? 1.12 : 1 / 1.12;
      setImageViewerZoom(imageViewerState.zoom * zoomFactor, pointerX, pointerY);
    },
    { passive: false },
  );
})();
