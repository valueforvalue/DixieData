(() => {
  const timers = new WeakMap();
  const redirectStateStorageKey = "dixiedata.redirectState";
  const toastStateStorageKey = "dixiedata.toastState";
  const backStackStorageKey = "dixiedata.backStack";
  const recentRecordsStorageKey = "dixiedata.recentRecords";
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

  function loadBackStack() {
    try {
      const raw = window.sessionStorage.getItem(backStackStorageKey);
      const parsed = raw ? JSON.parse(raw) : [];
      return Array.isArray(parsed) ? parsed : [];
    } catch (error) {
      return [];
    }
  }

  function saveBackStack(stack) {
    try {
      if (!Array.isArray(stack) || stack.length === 0) {
        window.sessionStorage.removeItem(backStackStorageKey);
        return;
      }
      window.sessionStorage.setItem(backStackStorageKey, JSON.stringify(stack.slice(-8)));
    } catch (error) {
      // Ignore storage failures and fall back to browser history.
    }
  }

  function loadRecentRecords() {
    try {
      const raw = window.localStorage.getItem(recentRecordsStorageKey);
      const parsed = raw ? JSON.parse(raw) : [];
      return Array.isArray(parsed) ? parsed.filter((value) => Number.isInteger(value) && value > 0) : [];
    } catch (error) {
      return [];
    }
  }

  function saveRecentRecords(ids) {
    try {
      const normalized = Array.from(new Set((Array.isArray(ids) ? ids : []).filter((value) => Number.isInteger(value) && value > 0))).slice(0, 10);
      if (normalized.length === 0) {
        window.localStorage.removeItem(recentRecordsStorageKey);
        return;
      }
      window.localStorage.setItem(recentRecordsStorageKey, JSON.stringify(normalized));
    } catch (error) {
      // Ignore storage failures.
    }
  }

  function rememberRecentRecordFromPage() {
    const detail = document.querySelector("[data-recent-record-id]");
    if (!(detail instanceof HTMLElement)) {
      return;
    }
    const id = Number.parseInt(detail.getAttribute("data-recent-record-id") || "", 10);
    if (!Number.isInteger(id) || id < 1) {
      return;
    }
    const next = [id].concat(loadRecentRecords().filter((value) => value !== id)).slice(0, 10);
    saveRecentRecords(next);
  }

  async function hydrateRecentSearchResults() {
    const emptyState = document.querySelector("[data-recent-records-empty]");
    const target = document.getElementById("soldier-list");
    const queryInput = document.querySelector('input[name="q"][hx-get="/soldiers/search"]');
    if (!(emptyState instanceof HTMLElement) || !(target instanceof HTMLElement) || !(queryInput instanceof HTMLInputElement)) {
      return;
    }
    if (queryInput.value.trim() !== "") {
      return;
    }
    const ids = loadRecentRecords();
    if (ids.length === 0) {
      return;
    }
    try {
      const response = await fetch(`/soldiers/search/recent?ids=${encodeURIComponent(ids.join(","))}`, {
        headers: {
          "X-Requested-With": "fetch",
        },
      });
      if (!response.ok) {
        return;
      }
      target.innerHTML = await response.text();
      initializeDynamicContent();
    } catch (error) {
      // Leave the empty state in place if the recent list cannot be loaded.
    }
  }

  function syncClonedFormState(root) {
    root.querySelectorAll("textarea").forEach((textarea) => {
      if (textarea instanceof HTMLTextAreaElement) {
        textarea.textContent = textarea.value;
      }
    });
    root.querySelectorAll("input").forEach((input) => {
      if (!(input instanceof HTMLInputElement)) {
        return;
      }
      if (input.type === "checkbox" || input.type === "radio") {
        if (input.checked) {
          input.setAttribute("checked", "checked");
        } else {
          input.removeAttribute("checked");
        }
        return;
      }
      input.setAttribute("value", input.value);
    });
    root.querySelectorAll("select").forEach((select) => {
      if (!(select instanceof HTMLSelectElement)) {
        return;
      }
      Array.from(select.options).forEach((option) => {
        option.selected = option.value === select.value;
        if (option.selected) {
          option.setAttribute("selected", "selected");
        } else {
          option.removeAttribute("selected");
        }
      });
    });
  }

  function currentViewSnapshot() {
    const main = document.querySelector("main");
    if (!(main instanceof HTMLElement)) {
      return null;
    }
    const clone = main.cloneNode(true);
    if (!(clone instanceof HTMLElement)) {
      return null;
    }
    syncClonedFormState(clone);
    return {
      path: `${window.location.pathname}${window.location.search}${window.location.hash}`,
      title: document.title,
      mainHTML: clone.innerHTML,
      scrollX: window.scrollX,
      scrollY: window.scrollY,
    };
  }

  function pushBackSnapshot() {
    const snapshot = currentViewSnapshot();
    if (!snapshot) {
      return;
    }
    const stack = loadBackStack();
    const previous = stack[stack.length - 1];
    if (previous && previous.path === snapshot.path && previous.mainHTML === snapshot.mainHTML) {
      return;
    }
    stack.push(snapshot);
    saveBackStack(stack);
  }

  function smartBackLabel(path) {
    const normalized = String(path || "").toLowerCase();
    if (normalized.startsWith("/calendar")) {
      return "Back to Calendar";
    }
    if (normalized.startsWith("/insights")) {
      return "Back to Insights";
    }
    if (normalized.startsWith("/share")) {
      return "Back to Share";
    }
    if (normalized.startsWith("/review-queue")) {
      return "Back to Review Queue";
    }
    if (normalized.startsWith("/soldiers/search") || normalized.startsWith("/soldiers?") || normalized === "/soldiers") {
      return "Back to Results";
    }
    return "Back";
  }

  function applySmartBackLabels() {
    const fallback = loadBackStack().slice(-1)[0];
    document.querySelectorAll("[data-history-back]").forEach((button) => {
      if (!(button instanceof HTMLButtonElement)) {
        return;
      }
      const fallbackLabel = button.getAttribute("data-fallback-label") || "Back";
      button.textContent = `← ${fallback ? smartBackLabel(fallback.path) : fallbackLabel}`;
    });
  }

  function restoreBackSnapshot() {
    const stack = loadBackStack();
    const snapshot = stack.pop();
    saveBackStack(stack);
    if (!snapshot) {
      return false;
    }
    const main = document.querySelector("main");
    if (!(main instanceof HTMLElement) || typeof snapshot.mainHTML !== "string") {
      return false;
    }
    document.title = snapshot.title || document.title;
    main.innerHTML = snapshot.mainHTML;
    window.history.replaceState(null, "", snapshot.path || window.location.pathname);
    initializeDynamicContent();
    window.requestAnimationFrame(() => {
      window.scrollTo({
        top: Number.isFinite(snapshot.scrollY) ? snapshot.scrollY : 0,
        left: Number.isFinite(snapshot.scrollX) ? snapshot.scrollX : 0,
        behavior: "auto",
      });
    });
    return true;
  }

  function shouldCaptureBackSnapshot(href) {
    const normalized = String(href || "");
    if (/^\/research-collections(?:\/\d+)?(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+\/research-pack\/(?:state|county)(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+\/conflict-ledger(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+\/research-log(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+\/timeline(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+\/camaraderie(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/\d+\/edit(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/soldiers\/new(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    if (/^\/compare(?:\?.*)?$/.test(normalized)) {
      return true;
    }
    return false;
  }

  function closestParentForm(el) {
    if (el instanceof HTMLFormElement) {
      return null;
    }
    return el.closest("form");
  }

  function ownerForm(el) {
    if (el instanceof HTMLFormElement) {
      return el;
    }
    const form = closestParentForm(el);
    return form instanceof HTMLFormElement ? form : null;
  }

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
    const form = closestParentForm(el);
    if (form) {
      return getMethod(form);
    }
    return "GET";
  }

  function getUrl(el, method) {
    const direct = el.getAttribute(`hx-${method.toLowerCase()}`);
    if (direct) {
      return direct;
    }
    const form = closestParentForm(el);
    return form ? getUrl(form, method) : null;
  }

  function getTarget(el) {
    const selector = el.getAttribute("hx-target");
    if (!selector) {
      const form = closestParentForm(el);
      return form ? getTarget(form) : document.body;
    }
    if (selector === "body") {
      return document.body;
    }
    return document.querySelector(selector);
  }

  function getSwap(el) {
    const form = closestParentForm(el);
    return el.getAttribute("hx-swap") || (form ? getSwap(form) : "innerHTML");
  }

  function syncPrimaryImageSelection(primaryImageId) {
    document.querySelectorAll("[data-image-card]").forEach((card) => {
      if (!(card instanceof HTMLElement)) {
        return;
      }
      const isPrimary = card.getAttribute("data-image-id") === String(primaryImageId);
      const badge = card.querySelector("[data-image-primary-badge]");
      const action = card.querySelector("[data-image-primary-action]");
      if (badge instanceof HTMLElement) {
        badge.classList.toggle("hidden", !isPrimary);
      }
      if (action instanceof HTMLElement) {
        action.classList.toggle("hidden", isPrimary);
      }
    });
  }

  function selectedCompareEntries(group) {
    return Array.from(document.querySelectorAll(`[data-checkbox-group="${group}"][data-compare-select]:checked`))
      .map((checkbox) => {
        if (!(checkbox instanceof HTMLInputElement)) {
          return null;
        }
        return {
          id: checkbox.value,
          label: checkbox.getAttribute("data-compare-label") || checkbox.value,
        };
      })
      .filter((entry) => entry && entry.id);
  }

  function selectedCompareIDs(group) {
    return selectedCompareEntries(group).map((entry) => entry.id);
  }

  function syncCompareSelectionUI(group) {
    const selected = selectedCompareEntries(group);
    const button = document.querySelector(`[data-compare-selected][data-compare-group="${group}"]`);
    const status = document.querySelector("[data-compare-selection-status]");
    if (button instanceof HTMLButtonElement) {
      const ready = selected.length === 2;
      button.disabled = !ready;
      button.classList.toggle("opacity-60", !ready);
      button.classList.toggle("cursor-not-allowed", !ready);
      button.textContent = ready ? `Compare Selected (${selected[0].label} vs ${selected[1].label})` : "Compare Selected";
    }
    if (status instanceof HTMLElement) {
      if (selected.length === 2) {
        status.textContent = `Ready to compare ${selected[0].label} and ${selected[1].label}.`;
      } else if (selected.length === 0) {
        status.textContent = "Select exactly two records to compare them side by side.";
      } else {
        status.textContent = `${selected[0].label} selected. Choose one more record to compare.`;
      }
    }
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
    const form = closestParentForm(el);
    if (form instanceof HTMLFormElement) {
      const payload = new FormData(form);
      if (el instanceof HTMLButtonElement || el instanceof HTMLInputElement) {
        if (el.name) {
          payload.append(el.name, el.value ?? "");
        }
      }
      return payload;
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
    viewer.className = "fixed inset-0 z-50 hidden items-center justify-center bg-[rgba(15,23,42,0.7)] p-6";
    viewer.innerHTML = `
      <div data-ui-id="overlay.image.viewer" class="relative flex max-h-full w-full max-w-6xl flex-col overflow-hidden rounded-[2rem] border border-[rgba(141,116,64,0.8)] bg-[rgba(36,48,61,0.97)] shadow-2xl">
        ${debugSurfaceIDsEnabled() ? '<div class="ui-debug-badge" aria-hidden="true">overlay.image.viewer</div>' : ""}
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-[rgba(141,116,64,0.35)] px-5 py-4 text-[#f4ead0]">
          <div>
            <p data-image-caption class="text-sm font-semibold tracking-[0.14em] text-[#eddca6]"></p>
            <p data-image-file class="mt-1 text-xs text-[rgba(244,234,208,0.72)]"></p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span data-image-zoom-label class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-xs text-[rgba(244,234,208,0.82)]">100%</span>
            <button type="button" data-image-rotate-ccw class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">Rotate CCW</button>
            <button type="button" data-image-rotate-cw class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">Rotate CW</button>
            <button type="button" data-image-zoom-out class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">-</button>
            <button type="button" data-image-zoom-in class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">+</button>
            <button type="button" data-image-reset class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">Reset</button>
            <button type="button" data-image-screenshot class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">Screenshot</button>
            <button type="button" data-image-close class="rounded-full border border-[rgba(141,116,64,0.5)] bg-[rgba(246,241,228,0.08)] px-3 py-1 text-sm text-[#f4ead0]">Close</button>
          </div>
        </div>
        <div data-image-stage class="relative h-[78vh] overflow-hidden bg-[rgba(21,29,38,0.96)]">
          <img data-image-element class="absolute left-1/2 top-1/2 max-w-none select-none rounded-2xl bg-white shadow-2xl" alt="" draggable="false" />
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3 border-t border-[rgba(141,116,64,0.35)] px-5 py-3 text-xs text-[rgba(244,234,208,0.68)]">
          <p>Mouse wheel zooms. Drag to move when zoomed in.</p>
          <p data-image-status></p>
        </div>
      </div>
    `;
    document.body.appendChild(viewer);
    return viewer;
  }

  // Quick preview keeps list triage in-context so people can inspect a record
  // without losing their place, filters, or compare selections.
  function ensurePreviewDrawer() {
    let drawer = document.getElementById("record-preview-drawer");
    if (drawer) {
      return drawer;
    }

    drawer = document.createElement("div");
    drawer.id = "record-preview-drawer";
    drawer.className = "fixed inset-0 z-[85] hidden bg-[rgba(15,23,42,0.45)]";
    drawer.innerHTML = `
      <div data-preview-backdrop class="absolute inset-0"></div>
      <aside class="absolute right-0 top-0 flex h-full w-full max-w-2xl flex-col overflow-hidden border-l border-[rgba(141,116,64,0.7)] bg-[rgba(246,241,228,0.98)] shadow-[-24px_0_60px_rgba(15,23,42,0.25)]">
        <div class="flex items-center justify-between gap-3 border-b border-[rgba(141,116,64,0.28)] px-5 py-4">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.24em] text-[#8d7440]">Quick View</p>
            <p class="mt-1 text-sm text-slate-600">Research notes, archive signals, and family context without leaving the results list.</p>
          </div>
          <button type="button" data-preview-close class="secondary-button px-4">Close</button>
        </div>
        <div data-preview-body class="flex-1 overflow-y-auto px-5 py-5"></div>
      </aside>
    `;
    document.body.appendChild(drawer);
    return drawer;
  }

  function closePreviewDrawer() {
    const drawer = document.getElementById("record-preview-drawer");
    if (!(drawer instanceof HTMLElement)) {
      return;
    }
    drawer.classList.add("hidden");
    document.body.classList.remove("overflow-hidden");
  }

  function openPreviewDrawer(targetId) {
    if (!targetId) {
      return;
    }
    const source = document.getElementById(targetId);
    if (!(source instanceof HTMLElement)) {
      showToast("Preview content was not available.", "error");
      return;
    }
    const drawer = ensurePreviewDrawer();
    const body = drawer.querySelector("[data-preview-body]");
    if (!(body instanceof HTMLElement)) {
      return;
    }
    body.innerHTML = source.innerHTML;
    body.scrollTop = 0;
    drawer.classList.remove("hidden");
    document.body.classList.add("overflow-hidden");
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
    const headingNode = target.querySelector("[data-record-persistence-heading]");
    const messageNode = target.querySelector("[data-record-persistence-message]");
    if (!(headingNode instanceof HTMLElement) || !(messageNode instanceof HTMLElement)) {
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
    headingNode.textContent = heading;
    messageNode.textContent = message;
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

  function discardDraftFromControl(control) {
    const form = ownerForm(control);
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    clearDraftForForm(form);
    const resetPath = control.getAttribute("data-clear-draft-path");
    if (resetPath) {
      window.location.assign(resetPath);
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
    const specialEntry = select.value === "wife" || select.value === "widow" || select.value === "linked_person";
    const widowEntry = select.value === "widow";
    const spouseEntry = select.value === "wife" || select.value === "widow";
    const linkedPersonEntry = select.value === "linked_person";
    form.querySelectorAll("[data-entry-type-special]").forEach((section) => {
      setSectionEnabled(section, specialEntry);
    });
    form.querySelectorAll("[data-spouse-only-field]").forEach((section) => {
      setSectionEnabled(section, spouseEntry);
    });
    form.querySelectorAll("[data-linked-person-field]").forEach((section) => {
      setSectionEnabled(section, linkedPersonEntry);
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
    return value !== "wife" && value !== "widow" && value !== "linked_person";
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

  function normalizedRedirectPath(path) {
    if (path === "/export") {
      return "/share";
    }
    return path;
  }

  function saveRedirectState(state) {
    try {
      window.sessionStorage.setItem(redirectStateStorageKey, JSON.stringify(state));
    } catch (error) {
      console.warn("Failed to persist redirect state", error);
    }
  }

  function clearRedirectState() {
    try {
      window.sessionStorage.removeItem(redirectStateStorageKey);
    } catch (error) {
      console.warn("Failed to clear redirect state", error);
    }
  }

  function loadRedirectState() {
    try {
      const raw = window.sessionStorage.getItem(redirectStateStorageKey);
      if (!raw) {
        return null;
      }
      return JSON.parse(raw);
    } catch (error) {
      console.warn("Failed to read redirect state", error);
      clearRedirectState();
      return null;
    }
  }

  function toastRegion() {
    const region = document.querySelector("[data-toast-region]");
    return region instanceof HTMLElement ? region : null;
  }

  function savePendingToast(state) {
    try {
      window.sessionStorage.setItem(toastStateStorageKey, JSON.stringify(state));
    } catch (error) {
      console.warn("Failed to persist toast state", error);
    }
  }

  function loadPendingToast() {
    try {
      const raw = window.sessionStorage.getItem(toastStateStorageKey);
      if (!raw) {
        return null;
      }
      window.sessionStorage.removeItem(toastStateStorageKey);
      return JSON.parse(raw);
    } catch (error) {
      console.warn("Failed to read toast state", error);
      return null;
    }
  }

  function showToast(message, kind = "success") {
    const region = toastRegion();
    if (!(region instanceof HTMLElement) || !message) {
      return;
    }
    const toast = document.createElement("div");
    toast.className = "toast-card";
    toast.setAttribute("data-toast-kind", kind);
    toast.innerHTML = `
      <div>
        <div class="text-xs font-semibold uppercase tracking-[0.22em] text-[#8d7440]">${kind === "error" ? "Attention" : "Success"}</div>
        <div class="mt-1 text-sm text-[#22303d]">${message}</div>
      </div>
      <button type="button" class="secondary-button px-3 py-1 text-xs" data-toast-dismiss>Dismiss</button>
    `;
    region.appendChild(toast);
    const dismiss = () => {
      toast.remove();
    };
    toast.querySelector("[data-toast-dismiss]")?.addEventListener("click", dismiss);
    window.setTimeout(dismiss, 4200);
  }

  function restorePendingToast() {
    const pending = loadPendingToast();
    if (!pending || !pending.message) {
      return;
    }
    showToast(pending.message, pending.kind || "success");
  }

  function rememberRedirectState(el, redirectTo, responseText, requestState) {
    const normalizedPath = normalizedRedirectPath(redirectTo);
    const redirectState = {
      path: normalizedPath,
      scrollX: requestState?.scrollX ?? window.scrollX,
      scrollY: requestState?.scrollY ?? window.scrollY,
      kind: "generic",
      message: "",
    };

    if (el.closest("[data-merge-review-item]")) {
      redirectState.kind = "merge-action";
    } else if (normalizedPath === "/share" && /conflicts?\s+staged\s+for\s+review/i.test(responseText)) {
      redirectState.kind = "merge-import";
      const conflictMatch = responseText.match(/(\d+)\s+conflicts?\s+staged\s+for\s+review/i);
      if (conflictMatch) {
        redirectState.message = `Data Loaded: ${conflictMatch[1]} Conflicts Found`;
      }
    }

    if (redirectState.kind === "generic" && normalizedPath !== window.location.pathname) {
      return;
    }

    saveRedirectState(redirectState);
  }

  function focusFirstMergeReviewAction() {
    const nextAction = document.querySelector("[data-merge-review-container] [data-merge-review-action]");
    if (nextAction instanceof HTMLElement) {
      const nextItem = nextAction.closest("[data-merge-review-item]");
      if (nextItem instanceof HTMLElement) {
        nextItem.scrollIntoView({ behavior: "smooth", block: "start" });
      }
      nextAction.focus({ preventScroll: true });
    }
  }

  function restoreRedirectState() {
    const redirectState = loadRedirectState();
    if (!redirectState || redirectState.path !== window.location.pathname) {
      return;
    }
    clearRedirectState();

    window.requestAnimationFrame(() => {
      window.scrollTo({
        top: Number.isFinite(redirectState.scrollY) ? redirectState.scrollY : window.scrollY,
        left: Number.isFinite(redirectState.scrollX) ? redirectState.scrollX : window.scrollX,
        behavior: "auto",
      });

      const mergeReviewContainer = document.getElementById("merge-review-section");
      const mergeReviewStatus = document.querySelector("[data-merge-review-loaded-status]");
      if (mergeReviewStatus instanceof HTMLElement && redirectState.message) {
        mergeReviewStatus.textContent = redirectState.message;
      }

      if (redirectState.kind === "merge-import" && mergeReviewContainer instanceof HTMLElement) {
        mergeReviewContainer.scrollIntoView({ behavior: "smooth", block: "start" });
        return;
      }

      if (redirectState.kind === "merge-action") {
        focusFirstMergeReviewAction();
      }
    });
  }

  function applyResponse(el, html, requestState) {
    const target = getTarget(el);
    if (!target) {
      return;
    }

    if (getSwap(el) === "none") {
      initializeDynamicContent();
      return;
    }

    const trimmed = html.trimStart().toLowerCase();
    if (target === document.body && trimmed.startsWith("<!doctype html")) {
      if (requestState) {
        saveRedirectState({
          path: window.location.pathname,
          scrollX: requestState.scrollX,
          scrollY: requestState.scrollY,
          kind: "generic",
          message: "",
        });
      }
      renderDocument(html);
      return;
    }

    if (target === document.body && trimmed.startsWith("<html")) {
      if (requestState) {
        saveRedirectState({
          path: window.location.pathname,
          scrollX: requestState.scrollX,
          scrollY: requestState.scrollY,
          kind: "generic",
          message: "",
        });
      }
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
    restoreRedirectState();
    restorePendingToast();
    applySmartBackLabels();
    rememberRecentRecordFromPage();
    hydrateRecentSearchResults();
  }

  function showProgress(el) {
    if (getSwap(el) === "none") {
      return;
    }
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
    const form = ownerForm(el);
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
    if (includesFiles && form instanceof HTMLFormElement && submitMultipartForm(form)) {
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
    const requestState = {
      scrollX: window.scrollX,
      scrollY: window.scrollY,
    };
    try {
      const response = await fetch(requestUrl, options);
      const html = await response.text();
      const redirectTo = response.headers.get("X-DixieData-Redirect");
      const toastMessage = response.headers.get("X-DixieData-Toast");
      const toastKind = response.headers.get("X-DixieData-Toast-Type") || "success";
      const closeFeedback = response.headers.get("X-DixieData-Close-Feedback");
      if (redirectTo) {
        if (form instanceof HTMLFormElement && response.ok) {
          clearDraftForForm(form);
        }
        if (toastMessage) {
          savePendingToast({ message: toastMessage, kind: toastKind });
        }
        rememberRedirectState(el, redirectTo, html, requestState);
        window.location.assign(redirectTo);
        return;
      }
      if (form instanceof HTMLFormElement && response.ok && response.redirected) {
        clearDraftForForm(form);
      }
      applyResponse(el, html, requestState);
      if (response.ok && closeFeedback === "true") {
        closeFeedbackModal();
      }
      if (response.ok && el instanceof HTMLElement) {
        const primaryImageId = el.getAttribute("data-primary-image-id");
        if (primaryImageId) {
          syncPrimaryImageSelection(primaryImageId);
        }
      }
      if (toastMessage) {
        showToast(toastMessage, toastKind);
      }
    } catch (error) {
      applyResponse(el, "Request failed.", requestState);
      showToast("Request failed.", "error");
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

  function feedbackModal() {
    const modal = document.querySelector("[data-feedback-modal]");
    return modal instanceof HTMLElement ? modal : null;
  }

  function feedbackForm() {
    const form = document.getElementById("feedback-form");
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

  function openFeedbackModal() {
    const modal = feedbackModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const form = feedbackForm();
    if (form instanceof HTMLFormElement) {
      const pathField = form.querySelector("[data-feedback-page-path]");
      if (pathField instanceof HTMLInputElement) {
        pathField.value = `${window.location.pathname || ""}${window.location.search || ""}`;
      }
    }
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    modal.setAttribute("aria-hidden", "false");
  }

  function closeFeedbackModal() {
    const modal = feedbackModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    modal.setAttribute("aria-hidden", "true");
    const form = feedbackForm();
    if (form instanceof HTMLFormElement) {
      form.reset();
      const status = document.getElementById("feedback-form-status");
      if (status instanceof HTMLElement) {
        status.textContent = "";
      }
    }
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
    restoreRedirectState();
    restorePendingToast();
    applySmartBackLabels();
    rememberRecentRecordFromPage();
    hydrateRecentSearchResults();
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
    const recordLink = event.target.closest("a[href]");
    if (recordLink instanceof HTMLAnchorElement && !event.defaultPrevented) {
      try {
        const target = new URL(recordLink.href, window.location.origin);
        if (target.origin === window.location.origin && shouldCaptureBackSnapshot(`${target.pathname}${target.search}`)) {
          pushBackSnapshot();
        }
      } catch (error) {
        // Ignore malformed URLs and continue with normal navigation.
      }
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
    const openFeedback = event.target.closest("[data-feedback-open]");
    if (openFeedback) {
      event.preventDefault();
      openFeedbackModal();
      return;
    }
    const closePrintConfig = event.target.closest("[data-print-config-close]");
    if (closePrintConfig) {
      event.preventDefault();
      closePrintConfigModal();
      return;
    }
    const closeFeedback = event.target.closest("[data-feedback-close]");
    if (closeFeedback) {
      event.preventDefault();
      closeFeedbackModal();
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
    const previewTrigger = event.target.closest("[data-preview-open]");
    if (previewTrigger instanceof HTMLElement) {
      event.preventDefault();
      openPreviewDrawer(previewTrigger.getAttribute("data-preview-target"));
      return;
    }
    if (event.target.closest("[data-preview-close],[data-preview-backdrop]")) {
      event.preventDefault();
      closePreviewDrawer();
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
    const clearDraft = event.target.closest("[data-clear-draft]");
    if (clearDraft instanceof HTMLElement) {
      event.preventDefault();
      discardDraftFromControl(clearDraft);
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
    const historyBack = event.target.closest("[data-history-back]");
    if (historyBack instanceof HTMLElement) {
      event.preventDefault();
      if (restoreBackSnapshot()) {
        return;
      }
      const fallbackHref = historyBack.getAttribute("data-fallback-href");
      if (fallbackHref) {
        window.location.assign(fallbackHref);
      } else if (window.history.length > 1) {
        window.history.back();
      } else {
        window.location.assign("/soldiers");
      }
      return;
    }
    const compareSelected = event.target.closest("[data-compare-selected]");
    if (compareSelected instanceof HTMLButtonElement) {
      event.preventDefault();
      const group = compareSelected.getAttribute("data-compare-group") || "search-compare";
      const selected = selectedCompareIDs(group);
      if (selected.length !== 2) {
        showToast("Choose exactly two records to compare.", "error");
        syncCompareSelectionUI(group);
        return;
      }
      pushBackSnapshot();
      window.location.assign(`/compare?id1=${encodeURIComponent(selected[0])}&id2=${encodeURIComponent(selected[1])}`);
      return;
    }
    const mergeReviewAction = event.target.closest("[data-merge-review-action]");
    if (mergeReviewAction) {
      event.preventDefault();
      request(mergeReviewAction);
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
    request(event.submitter instanceof HTMLElement ? event.submitter : form);
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
  document.addEventListener("change", (event) => {
    const compareSelect = event.target.closest("[data-compare-select]");
    if (!(compareSelect instanceof HTMLInputElement)) {
      return;
    }
    const group = compareSelect.getAttribute("data-checkbox-group") || "search-compare";
    syncCompareSelectionUI(group);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closePrintConfigModal();
      closeFeedbackModal();
      closeTextContextMenu();
      closeImageViewer();
      closePreviewDrawer();
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
    const feedback = feedbackModal();
    if (feedback && event.target === feedback) {
      closeFeedbackModal();
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
