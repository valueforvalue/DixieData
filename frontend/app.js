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
  const textContextMenuState = {
    target: null,
    selectionText: "",
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

  function isTextInputTarget(target) {
    if (!(target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement)) {
      return false;
    }
    if (target instanceof HTMLTextAreaElement) {
      return true;
    }
    const type = (target.type || "text").toLowerCase();
    return ["text", "search", "url", "tel", "email", "password", "number"].includes(type);
  }

  function isEditableTextTarget(target) {
    return isTextInputTarget(target) || target instanceof HTMLElement && target.isContentEditable;
  }

  function textSelectionLength(target) {
    if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
      const start = typeof target.selectionStart === "number" ? target.selectionStart : 0;
      const end = typeof target.selectionEnd === "number" ? target.selectionEnd : 0;
      return Math.max(0, end - start);
    }
    const selection = window.getSelection();
    return selection ? selection.toString().length : 0;
  }

  function ensureTextContextMenu() {
    let menu = document.getElementById("text-context-menu");
    if (menu) {
      return menu;
    }

    menu = document.createElement("div");
    menu.id = "text-context-menu";
    menu.className = "fixed hidden min-w-[12rem] rounded-2xl border border-[rgba(141,116,64,0.8)] bg-[rgba(246,241,228,0.98)] p-2 shadow-[0_18px_50px_rgba(23,33,43,0.28)]";
    menu.style.zIndex = "95";
    menu.innerHTML = `
      <button type="button" data-text-menu-action="cut" class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-[#22303d] hover:bg-[rgba(36,48,61,0.08)]">Cut</button>
      <button type="button" data-text-menu-action="copy" class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-[#22303d] hover:bg-[rgba(36,48,61,0.08)]">Copy</button>
      <button type="button" data-text-menu-action="paste" class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-[#22303d] hover:bg-[rgba(36,48,61,0.08)]">Paste</button>
      <button type="button" data-text-menu-action="select-all" class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-[#22303d] hover:bg-[rgba(36,48,61,0.08)]">Select All</button>
    `;
    document.body.appendChild(menu);
    return menu;
  }

  function closeTextContextMenu() {
    const menu = document.getElementById("text-context-menu");
    if (menu) {
      menu.classList.add("hidden");
    }
    textContextMenuState.target = null;
    textContextMenuState.selectionText = "";
  }

  function updateTextContextMenuState() {
    const menu = ensureTextContextMenu();
    const target = textContextMenuState.target;
    const editable = isEditableTextTarget(target);
    const selectionLen = target ? textSelectionLength(target) : 0;
    const canCut = editable && selectionLen > 0 && !target.readOnly && !target.disabled;
    const canCopy = selectionLen > 0 || !!textContextMenuState.selectionText;
    const canPaste = editable && !target.readOnly && !target.disabled;
    const canSelectAll = editable || !!textContextMenuState.selectionText;

    menu.querySelectorAll("[data-text-menu-action]").forEach((button) => {
      if (!(button instanceof HTMLButtonElement)) {
        return;
      }
      const action = button.getAttribute("data-text-menu-action");
      let enabled = false;
      switch (action) {
        case "cut":
          enabled = canCut;
          break;
        case "copy":
          enabled = canCopy;
          break;
        case "paste":
          enabled = canPaste;
          break;
        case "select-all":
          enabled = canSelectAll;
          break;
      }
      button.disabled = !enabled;
      button.classList.toggle("opacity-50", !enabled);
      button.classList.toggle("cursor-not-allowed", !enabled);
    });
  }

  function openTextContextMenu(target, clientX, clientY) {
    const menu = ensureTextContextMenu();
    textContextMenuState.target = target;
    textContextMenuState.selectionText = (window.getSelection()?.toString() || "").trim();
    updateTextContextMenuState();
    menu.classList.remove("hidden");

    const { innerWidth, innerHeight } = window;
    const rect = menu.getBoundingClientRect();
    const left = Math.min(clientX, innerWidth - rect.width - 8);
    const top = Math.min(clientY, innerHeight - rect.height - 8);
    menu.style.left = `${Math.max(8, left)}px`;
    menu.style.top = `${Math.max(8, top)}px`;
  }

  function replaceTextSelection(target, replacement) {
    if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
      const start = typeof target.selectionStart === "number" ? target.selectionStart : target.value.length;
      const end = typeof target.selectionEnd === "number" ? target.selectionEnd : target.value.length;
      const value = target.value || "";
      target.value = `${value.slice(0, start)}${replacement}${value.slice(end)}`;
      const caret = start + replacement.length;
      target.setSelectionRange(caret, caret);
      target.dispatchEvent(new Event("input", { bubbles: true }));
      return;
    }
    if (target instanceof HTMLElement && target.isContentEditable) {
      document.execCommand("insertText", false, replacement);
    }
  }

  async function performTextContextMenuAction(action) {
    const target = textContextMenuState.target;
    if (!target) {
      closeTextContextMenu();
      return;
    }
    if (target instanceof HTMLElement) {
      target.focus();
    }

    switch (action) {
      case "cut":
        document.execCommand("cut");
        break;
      case "copy":
        document.execCommand("copy");
        break;
      case "paste":
        try {
          if (navigator.clipboard?.readText) {
            replaceTextSelection(target, await navigator.clipboard.readText());
          } else {
            document.execCommand("paste");
          }
        } catch (error) {
          document.execCommand("paste");
        }
        break;
      case "select-all":
        if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
          target.select();
        } else if (target instanceof HTMLElement && target.isContentEditable) {
          const range = document.createRange();
          range.selectNodeContents(target);
          const selection = window.getSelection();
          selection?.removeAllRanges();
          selection?.addRange(range);
        }
        break;
    }

    closeTextContextMenu();
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

  function pageScratchpadDisplayId() {
    const explicit = document.querySelector("[data-scratchpad-display-id]");
    if (explicit instanceof HTMLElement) {
      const value = (explicit.getAttribute("data-scratchpad-display-id") || "").trim();
      if (value) {
        return value;
      }
    }
    const fields = document.querySelectorAll('input[name="display_id"]');
    for (const field of fields) {
      if (field instanceof HTMLInputElement) {
        const value = (field.value || "").trim();
        if (value) {
          return value;
        }
      }
    }
    return "";
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
    return null;
  }

  function normalizeScratchpadDisplayId(displayId) {
    const value = (displayId || "").trim();
    return value || "unfiled";
  }

  function scratchpadContentKey(displayId) {
    return `dixiedata:scratchpad:${normalizeScratchpadDisplayId(displayId)}`;
  }

  function loadLegacyScratchpadText(displayId) {
    try {
      return window.localStorage.getItem(scratchpadContentKey(displayId)) || "";
    } catch (error) {
      return "";
    }
  }

  function scratchpadStatusTarget(trigger) {
    if (!(trigger instanceof HTMLElement)) {
      const globalTarget = document.querySelector("[data-floating-scratchpad-status]");
      return globalTarget instanceof HTMLElement ? globalTarget : null;
    }
    const section = trigger.closest("[data-ui-id='panel.soldier.form.scratchpad']");
    if (section instanceof HTMLElement) {
      const target = section.querySelector("[data-scratchpad-status]");
      if (target instanceof HTMLElement) {
        return target;
      }
    }
    const globalTarget = document.querySelector("[data-floating-scratchpad-status]");
    return globalTarget instanceof HTMLElement ? globalTarget : null;
  }

  function setScratchpadStatus(trigger, message, isError = false) {
    const target = scratchpadStatusTarget(trigger);
    if (!(target instanceof HTMLElement)) {
      return;
    }
    target.textContent = message || "";
    target.classList.toggle("text-red-700", isError);
    target.classList.toggle("text-slate-500", !isError);
  }

  async function openScratchpad(trigger) {
    const form = scratchpadFormFromElement(trigger);
    const displayId = scratchpadDisplayId(form) || pageScratchpadDisplayId();
    if (!displayId) {
      setScratchpadStatus(trigger, "Open a record with a saved Record ID before launching the scratch pad.", true);
      return;
    }
    const data = form instanceof HTMLFormElement ? new FormData(form) : new FormData();
    data.set("display_id", displayId);
    const legacyText = loadLegacyScratchpadText(displayId);
    if (legacyText) {
      data.set("scratchpad_seed", legacyText);
    }
    setBusyState(trigger, true);
    try {
      const response = await fetch("/scratchpad/open", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
          "X-Requested-With": "DixieData"
        },
        body: toSearchParams(data).toString(),
      });
      const message = await response.text();
      setScratchpadStatus(trigger, message || "Scratch pad opened.", !response.ok);
    } catch (error) {
      setScratchpadStatus(trigger, "Scratch pad failed to open.", true);
    } finally {
      setBusyState(trigger, false);
    }
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

  function isDraftableField(field) {
    if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement || field instanceof HTMLSelectElement)) {
      return false;
    }
    if (!field.name || field.disabled) {
      return false;
    }
    if (field instanceof HTMLInputElement && (field.type === "file" || field.readOnly)) {
      return false;
    }
    return true;
  }

  function persistDraftForForm(form) {
    const key = draftKeyForForm(form);
    if (!key) {
      return;
    }
    const payload = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!isDraftableField(field)) {
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

  function recordPersistenceTarget(form) {
    if (!(form instanceof HTMLFormElement)) {
      return null;
    }
    const target = form.querySelector("[data-record-persistence]");
    return target instanceof HTMLElement ? target : null;
  }

  function syncRecordPersistenceIndicator(form, hasDraft, restored = false) {
    const target = recordPersistenceTarget(form);
    if (!(target instanceof HTMLElement)) {
      return;
    }
    const kind = target.getAttribute("data-record-persistence-kind") || "new";
    target.classList.remove("border-emerald-700/40", "bg-emerald-50/80", "text-emerald-900", "border-amber-700/40", "bg-amber-50/80", "text-amber-900");
    let heading = "";
    let message = "";
    if (kind === "edit" && !hasDraft) {
      heading = "Committed to database.";
      message = "This record currently matches the primary database.";
      target.classList.add("border-emerald-700/40", "bg-emerald-50/80", "text-emerald-900");
    } else if (kind === "edit") {
      heading = restored ? "Local draft restored." : "Unsaved local edits.";
      message = "Your current changes are cached in localStorage and have not been committed to the database yet.";
      target.classList.add("border-amber-700/40", "bg-amber-50/80", "text-amber-900");
    } else {
      heading = restored ? "Local draft restored." : "Local draft only.";
      message = "This new record exists only in localStorage until you create it in the database.";
      target.classList.add("border-amber-700/40", "bg-amber-50/80", "text-amber-900");
    }
    target.innerHTML = `<strong class="font-semibold">${heading}</strong><span class="ml-2">${message}</span>`;
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
    syncRecordPersistenceIndicator(form, false);
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
      syncRecordPersistenceIndicator(form, false);
      return;
    }
    if (!saved) {
      syncRecordPersistenceIndicator(form, false);
      return;
    }
    let payload;
    try {
      payload = JSON.parse(saved);
    } catch (error) {
      syncRecordPersistenceIndicator(form, false);
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
      if (!isDraftableField(field)) {
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
    syncRecordPersistenceIndicator(form, true, true);
  }

  function initializeDraftForms() {
    document.querySelectorAll("form[data-draft-key]").forEach((form) => {
      if (form instanceof HTMLFormElement) {
        restoreDraftForForm(form);
      }
    });
  }

  function initializeFloatingNav() {
    const panel = document.querySelector("[data-floating-nav-panel]");
    const toggle = document.querySelector("[data-floating-nav-toggle]");
    if (!(panel instanceof HTMLElement) || !(toggle instanceof HTMLButtonElement)) {
      return;
    }
    toggle.addEventListener("click", (event) => {
      event.preventDefault();
      panel.classList.toggle("hidden");
    });
    document.addEventListener("click", (event) => {
      if (panel.classList.contains("hidden")) {
        return;
      }
      if (event.target instanceof Node && !panel.contains(event.target) && !toggle.contains(event.target)) {
        panel.classList.add("hidden");
      }
    });
  }

  function setSectionEnabled(section, enabled) {
    if (!(section instanceof HTMLElement)) {
      return;
    }
    section.classList.toggle("hidden", !enabled);
    section.querySelectorAll("input, select, textarea").forEach((field) => {
      if (field instanceof HTMLInputElement || field instanceof HTMLSelectElement || field instanceof HTMLTextAreaElement) {
        field.disabled = !enabled;
      }
    });
  }

  function syncEntryTypeFields(form) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const select = form.querySelector("[data-entry-type-select]");
    if (!(select instanceof HTMLSelectElement)) {
      return;
    }
    const specialEntry = select.value === "wife" || select.value === "widow";
    const widowEntry = select.value === "widow";
    form.querySelectorAll("[data-entry-type-special]").forEach((section) => {
      setSectionEnabled(section, specialEntry);
    });
    form.querySelectorAll("[data-soldier-only-field]").forEach((section) => {
      setSectionEnabled(section, isSoldierEntryType(select.value));
    });
    form.querySelectorAll("[data-soldier-or-widow-field]").forEach((section) => {
      setSectionEnabled(section, isSoldierEntryType(select.value) || widowEntry);
    });
    syncConfederateHomeFields(form);
  }

  function isSoldierEntryType(value) {
    return value !== "wife" && value !== "widow";
  }

  function initializeEntryTypeForms() {
    document.querySelectorAll("form").forEach((form) => {
      syncEntryTypeFields(form);
    });
  }

  function syncConfederateHomeFields(form) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const status = form.querySelector("[data-confederate-home-status]");
    const nameField = form.querySelector("[data-confederate-home-name-field] input[name='confederate_home_name']");
    const nameWrapper = form.querySelector("[data-confederate-home-name-field]");
    if (!(status instanceof HTMLSelectElement) || !(nameField instanceof HTMLInputElement)) {
      return;
    }
    const enabled = status.value !== "None";
    nameField.disabled = !enabled;
    if (!enabled) {
      nameField.value = "";
    }
    if (nameWrapper instanceof HTMLElement) {
      nameWrapper.classList.toggle("opacity-60", !enabled);
    }
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
      initializeDynamicContent();
      return;
    }

    target.innerHTML = html;
    initializeDynamicContent();
  }

  function initializeDynamicContent() {
    initializeTabs();
    initializeEntryTypeForms();
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

  function printConfigModal() {
    const modal = document.querySelector("[data-print-config-modal]");
    return modal instanceof HTMLElement ? modal : null;
  }

  function printConfigForm() {
    const form = document.getElementById("share-print-config-form");
    return form instanceof HTMLFormElement ? form : null;
  }

  function openPrintConfigModal() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    modal.setAttribute("aria-hidden", "false");
  }

  function closePrintConfigModal() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    modal.setAttribute("aria-hidden", "true");
  }

  function readPrintSettings(form) {
    return {
      sortBy: (form.querySelector('input[name="sort_by"]:checked')?.value || "last_name").trim(),
      groupByUnit: form.querySelector('input[name="group_by_unit"]')?.checked === true,
      groupByPensionState: form.querySelector('input[name="group_by_pension_state"]')?.checked === true,
      groupByConfederateHomeStatus: form.querySelector('input[name="group_by_confederate_home_status"]')?.checked === true,
      groupByBuriedIn: form.querySelector('input[name="group_by_buried_in"]')?.checked === true,
    };
  }

  function shareStatusTarget() {
    const target = document.getElementById("share-status");
    return target instanceof HTMLElement ? target : null;
  }

  async function submitPrintConfig(form, trigger) {
    const bridge = window.go?.main?.App?.ExportFullDatabasePDF;
    if (typeof bridge !== "function") {
      closePrintConfigModal();
      request(form);
      return;
    }

    const status = shareStatusTarget();
    const settings = readPrintSettings(form);
    setBusyState(trigger, true);
    if (status) {
      status.textContent = "Preparing printable PDF...";
    }
    try {
      const markup = await bridge(settings);
      closePrintConfigModal();
      if (status) {
        status.innerHTML = markup || "Printable PDF export finished.";
      }
    } catch (error) {
      if (status) {
        status.textContent = `Printable PDF export failed: ${error?.message || error || "unknown error"}`;
      }
    } finally {
      setBusyState(trigger, false);
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    initializeTabs();
    initializeDraftForms();
    initializeEntryTypeForms();
    initializeFloatingNav();
    document.querySelectorAll('[hx-trigger="load"]').forEach((el) => {
      request(el);
    });
  });

  document.addEventListener("click", (event) => {
    const textMenuAction = event.target.closest("[data-text-menu-action]");
    if (textMenuAction instanceof HTMLButtonElement) {
      event.preventDefault();
      performTextContextMenuAction(textMenuAction.getAttribute("data-text-menu-action"));
      return;
    }
    if (!event.target.closest("#text-context-menu")) {
      closeTextContextMenu();
    }
    const externalLink = event.target.closest("a[data-open-external]");
    if (externalLink instanceof HTMLAnchorElement) {
      event.preventDefault();
      openExternalLinkInChrome(externalLink.href);
      return;
    }
    const openPrintConfig = event.target.closest("[data-print-config-open]");
    if (openPrintConfig) {
      event.preventDefault();
      openPrintConfigModal();
      return;
    }
    const closePrintConfig = event.target.closest("[data-print-config-close]");
    if (closePrintConfig) {
      event.preventDefault();
      closePrintConfigModal();
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
        syncRecordPersistenceIndicator(form, true);
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
        syncRecordPersistenceIndicator(form, true);
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
    if (form.id === "share-print-config-form") {
      event.preventDefault();
      submitPrintConfig(form, event.submitter instanceof HTMLElement ? event.submitter : form);
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
    const form = event.target.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      persistDraftForForm(form);
      syncRecordPersistenceIndicator(form, true);
    }
  });
  document.addEventListener("change", (event) => {
    const form = event.target.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      persistDraftForForm(form);
      syncRecordPersistenceIndicator(form, true);
    }
  });
  document.addEventListener("change", (event) => {
    const entryTypeSelect = event.target.closest("[data-entry-type-select]");
    if (entryTypeSelect) {
      const form = entryTypeSelect.closest("form");
      if (form instanceof HTMLFormElement) {
        syncEntryTypeFields(form);
      }
    }
  });
  document.addEventListener("change", (event) => {
    const homeStatusSelect = event.target.closest("[data-confederate-home-status]");
    if (homeStatusSelect) {
      const form = homeStatusSelect.closest("form");
      if (form instanceof HTMLFormElement) {
        syncConfederateHomeFields(form);
      }
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
      closePrintConfigModal();
      closeTextContextMenu();
      closeImageViewer();
      const panel = document.querySelector("[data-floating-nav-panel]");
      if (panel instanceof HTMLElement) {
        panel.classList.add("hidden");
      }
    }
  });
  document.addEventListener("contextmenu", (event) => {
    const editableTarget = event.target.closest("input, textarea, [contenteditable='true'], [contenteditable=''], [contenteditable='plaintext-only']");
    const hasTextSelection = (window.getSelection()?.toString() || "").trim() !== "";
    if (!(editableTarget instanceof HTMLElement) && !hasTextSelection) {
      closeTextContextMenu();
      return;
    }
    if (editableTarget instanceof HTMLElement && !isEditableTextTarget(editableTarget)) {
      if (!hasTextSelection) {
        closeTextContextMenu();
        return;
      }
    }
    event.preventDefault();
    openTextContextMenu(editableTarget instanceof HTMLElement ? editableTarget : document.activeElement, event.clientX, event.clientY);
  });
  document.addEventListener("mousedown", (event) => {
    const modal = printConfigModal();
    if (modal && event.target === modal) {
      closePrintConfigModal();
      return;
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
    stopImageViewerDrag();
  });
  document.addEventListener("mouseleave", () => {
    stopImageViewerDrag();
  });
  window.addEventListener("blur", () => {
    stopImageViewerDrag();
  });
  window.addEventListener("resize", () => {
    const viewer = document.getElementById("image-viewer");
    if (viewer && !viewer.classList.contains("hidden")) {
      resetImageViewerTransform();
    }
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
