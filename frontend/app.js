(() => {
  const timers = new WeakMap();
  const redirectStateStorageKey = "dixiedata.redirectState";
  const toastStateStorageKey = "dixiedata.toastState";
  const deletedDraftStateStorageKey = "dixiedata.deletedDraftState";
  const backStackStorageKey = "dixiedata.backStack";
  const recentRecordsStorageKey = "dixiedata.recentRecords";
  const browseStateStorageKey = "dixiedata.browse.state";
  const browseColumnsStorageKey = "dixiedata.browse.columns";
  const browseSelectionStorageKey = "dixiedata.browse.selection";
  const calendarAnniversaryDensityStorageKey = "dixiedata.calendar.anniversaryDensity";
  const layoutModeStorageKey = "dixiedata.layout.mode";
  const pdfPreferencesStoragePrefix = "dixiedata.pdfPrefs.";
  const splitScreenBreakpointPx = 1000;
  // Toast auto-dismiss timing. success + info kinds fade out after
  // this delay; warning + error stay until the user clicks Dismiss
  // (issue #54 contract). Tuning this constant is the single source
  // of truth for both the inline showToast call and the
  // sessionStorage-restore path.
  const toastAutoDismissMs = 4000;
  // Toast fade-out animation length, kept in sync with the
  // .toast-card CSS opacity transition + the remove() defer in
  // showToast's dismiss helper. Set to the visible fade duration
  // so the DOM node is removed only after the animation completes.
  const toastFadeOutMs = 320;
  const recentSearchHydrationState = { token: 0 };
  const defaultBrowseColumns = ["display_id", "name", "entry_type", "rank_out", "unit", "pension_state", "review_status", "last_edited"];
  const draftBaselines = new WeakMap();
  const staleDrafts = new WeakMap();
  let layoutModeMediaQuery = null;
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

  function loadJSONStorage(key, fallback) {
    try {
      const raw = window.localStorage.getItem(key);
      if (!raw) {
        return fallback;
      }
      const parsed = JSON.parse(raw);
      return parsed ?? fallback;
    } catch (error) {
      return fallback;
    }
  }

  function saveJSONStorage(key, value) {
    try {
      if (value == null) {
        window.localStorage.removeItem(key);
        return;
      }
      window.localStorage.setItem(key, JSON.stringify(value));
    } catch (error) {
      // Ignore storage failures.
    }
  }

  function loadBrowseState() {
    const value = loadJSONStorage(browseStateStorageKey, {});
    return value && typeof value === "object" ? value : {};
  }

  function saveBrowseState(state) {
    saveJSONStorage(browseStateStorageKey, state);
  }

  function loadBrowseColumns() {
    const value = loadJSONStorage(browseColumnsStorageKey, defaultBrowseColumns);
    return Array.isArray(value) && value.length > 0 ? value : defaultBrowseColumns.slice();
  }

  function saveBrowseColumns(columns) {
    const normalized = Array.from(new Set((Array.isArray(columns) ? columns : []).filter((value) => typeof value === "string" && value !== "")));
    saveJSONStorage(browseColumnsStorageKey, normalized.length > 0 ? normalized : defaultBrowseColumns);
  }

  function loadBrowseSelection() {
    const value = loadJSONStorage(browseSelectionStorageKey, []);
    return Array.isArray(value) ? value.filter((entry) => Number.isInteger(entry) && entry > 0) : [];
  }

  function loadCalendarAnniversaryDensity() {
    const value = loadJSONStorage(calendarAnniversaryDensityStorageKey, "expanded");
    return value === "compact" ? "compact" : "expanded";
  }

  function saveCalendarAnniversaryDensity(mode) {
    saveJSONStorage(calendarAnniversaryDensityStorageKey, mode === "compact" ? "compact" : "expanded");
  }

  function loadLayoutModePreference() {
    const value = loadJSONStorage(layoutModeStorageKey, "auto");
    return value === "relaxed" || value === "split-screen" ? value : "auto";
  }

  function saveLayoutModePreference(mode) {
    const normalized = mode === "relaxed" || mode === "split-screen" ? mode : "auto";
    saveJSONStorage(layoutModeStorageKey, normalized);
    return normalized;
  }

  function resolveResponsiveLayoutMode(preference) {
    if (preference === "relaxed" || preference === "split-screen") {
      return preference;
    }
    if (typeof window.matchMedia === "function") {
      return window.matchMedia(`(max-width: ${splitScreenBreakpointPx}px)`).matches ? "split-screen" : "relaxed";
    }
    return window.innerWidth <= splitScreenBreakpointPx ? "split-screen" : "relaxed";
  }

  function layoutModeLabel(mode) {
    return mode === "split-screen" ? "Split-screen" : "Relaxed";
  }

  function layoutPreferenceLabel(preference) {
    switch (preference) {
      case "relaxed":
        return "Manual relaxed";
      case "split-screen":
        return "Manual split-screen";
      default:
        return "Auto";
    }
  }

  function refreshResponsiveLayoutControls(root, preference, mode) {
    const scope = root && root.nodeType === 9 ? root : document;
    scope.querySelectorAll("[data-layout-mode-option]").forEach((button) => {
      if (!(button instanceof HTMLButtonElement)) {
        return;
      }
      const value = button.getAttribute("data-layout-mode-option") || "auto";
      const active = value === preference;
      button.setAttribute("data-layout-mode-active", active ? "true" : "false");
      button.setAttribute("aria-pressed", active ? "true" : "false");
    });
    scope.querySelectorAll("[data-layout-mode-status]").forEach((node) => {
      node.textContent = layoutModeLabel(mode);
    });
    scope.querySelectorAll("[data-layout-mode-preference-label]").forEach((node) => {
      node.textContent = layoutPreferenceLabel(preference);
    });
    scope.querySelectorAll("[data-layout-mode-breakpoint]").forEach((node) => {
      node.textContent = `${splitScreenBreakpointPx}px`;
    });
  }

  function applyResponsiveLayout(root = document) {
    const doc = root && root.nodeType === 9 ? root : document;
    const body = doc.body;
    const html = doc.documentElement || document.documentElement;
    if (!(body instanceof HTMLElement) || !(html instanceof HTMLElement)) {
      return;
    }
    const preference = loadLayoutModePreference();
    const mode = resolveResponsiveLayoutMode(preference);
    html.setAttribute("data-layout-mode", mode);
    html.setAttribute("data-layout-mode-preference", preference);
    body.setAttribute("data-layout-mode", mode);
    body.setAttribute("data-layout-mode-preference", preference);
    refreshResponsiveLayoutControls(doc, preference, mode);
    clampPopoutPanels(doc);
  }

  function clampPopoutPanels(root = document) {
    const scope = root && root.nodeType === 9 ? root : document;
    const viewportPadding = 12;
    scope.querySelectorAll("[data-popout-panel]").forEach((panel) => {
      if (!(panel instanceof HTMLElement)) {
        return;
      }
      const detailsHost = panel.closest("details");
      if (detailsHost && detailsHost.tagName === "DETAILS" && !detailsHost.open) {
        panel.style.removeProperty("transform");
        return;
      }
      if (panel.offsetParent === null) {
        panel.style.removeProperty("transform");
        return;
      }
      panel.style.removeProperty("transform");
      const rect = panel.getBoundingClientRect();
      let shiftX = 0;
      if (rect.right > window.innerWidth - viewportPadding) {
        shiftX -= rect.right - (window.innerWidth - viewportPadding);
      }
      if (rect.left + shiftX < viewportPadding) {
        shiftX += viewportPadding - (rect.left + shiftX);
      }
      if (Math.abs(shiftX) > 0.5) {
        panel.style.transform = `translateX(${Math.round(shiftX)}px)`;
        return;
      }
      panel.style.removeProperty("transform");
    });
  }

  function ensureResponsiveLayoutWatcher() {
    if (layoutModeMediaQuery || typeof window.matchMedia !== "function") {
      return;
    }
    layoutModeMediaQuery = window.matchMedia(`(max-width: ${splitScreenBreakpointPx}px)`);
    const handleChange = () => {
      if (loadLayoutModePreference() === "auto") {
        applyResponsiveLayout(document);
      }
    };
    if (typeof layoutModeMediaQuery.addEventListener === "function") {
      layoutModeMediaQuery.addEventListener("change", handleChange);
      return;
    }
    if (typeof layoutModeMediaQuery.addListener === "function") {
      layoutModeMediaQuery.addListener(handleChange);
    }
  }

  function saveBrowseSelection(ids) {
    const normalized = Array.from(new Set((Array.isArray(ids) ? ids : []).filter((value) => Number.isInteger(value) && value > 0)));
    saveJSONStorage(browseSelectionStorageKey, normalized);
  }

  function loadPDFPreferences(scope) {
    if (!scope) {
      return {};
    }
    const value = loadJSONStorage(`${pdfPreferencesStoragePrefix}${scope}`, {});
    return value && typeof value === "object" ? value : {};
  }

  function savePDFPreferences(scope, values) {
    if (!scope) {
      return;
    }
    saveJSONStorage(`${pdfPreferencesStoragePrefix}${scope}`, values);
  }

  function pdfPreferenceValue(input) {
    if (input instanceof HTMLInputElement && input.type === "checkbox") {
      return input.checked;
    }
    return input.value;
  }

  function applyPDFPreferences(form) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scope = form.getAttribute("data-pdf-pref-scope");
    const prefs = loadPDFPreferences(scope);
    form.querySelectorAll("[data-pdf-pref-key]").forEach((input) => {
      if (!(input instanceof HTMLInputElement || input instanceof HTMLSelectElement)) {
        return;
      }
      const key = input.getAttribute("data-pdf-pref-key") || "";
      if (!(key in prefs)) {
        return;
      }
      if (input instanceof HTMLInputElement && input.type === "checkbox") {
        input.checked = Boolean(prefs[key]);
        return;
      }
      input.value = String(prefs[key]);
    });
  }

  function persistPDFPreferences(form) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scope = form.getAttribute("data-pdf-pref-scope");
    const next = {};
    form.querySelectorAll("[data-pdf-pref-key]").forEach((input) => {
      if (!(input instanceof HTMLInputElement || input instanceof HTMLSelectElement)) {
        return;
      }
      const key = input.getAttribute("data-pdf-pref-key");
      if (!key) {
        return;
      }
      next[key] = pdfPreferenceValue(input);
    });
    savePDFPreferences(scope, next);
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

  function quickSearchInput() {
    const input = document.querySelector('input[name="q"][hx-get="/soldiers/search"]');
    return input instanceof HTMLInputElement ? input : null;
  }

  function invalidateRecentSearchHydration() {
    recentSearchHydrationState.token += 1;
  }

  async function hydrateRecentSearchResults() {
    const emptyState = document.querySelector("[data-recent-records-empty]");
    const target = document.getElementById("soldier-list");
    const queryInput = quickSearchInput();
    if (!(emptyState instanceof HTMLElement) || !(target instanceof HTMLElement) || !(queryInput instanceof HTMLInputElement)) {
      return;
    }
    if (queryInput.value.trim() !== "" || document.activeElement === queryInput) {
      return;
    }
    const ids = loadRecentRecords();
    if (ids.length === 0) {
      return;
    }
    const token = recentSearchHydrationState.token + 1;
    recentSearchHydrationState.token = token;
    try {
      const response = await fetch(`/soldiers/search/recent?ids=${encodeURIComponent(ids.join(","))}`, {
        headers: {
          "X-Requested-With": "fetch",
        },
      });
      if (!response.ok || recentSearchHydrationState.token !== token) {
        return;
      }
      const html = await response.text();
      if (recentSearchHydrationState.token !== token) {
        return;
      }
      const liveQueryInput = quickSearchInput();
      const liveTarget = document.getElementById("soldier-list");
      if (!(liveQueryInput instanceof HTMLInputElement) || !(liveTarget instanceof HTMLElement)) {
        return;
      }
      if (liveQueryInput.value.trim() !== "" || document.activeElement === liveQueryInput) {
        return;
      }
      liveTarget.innerHTML = html;
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
    menu.className = "fixed hidden min-w-[12rem] max-w-[calc(100vw-1rem)] rounded-2xl border border-[rgba(141,116,64,0.8)] bg-[rgba(246,241,228,0.98)] p-2 shadow-[0_18px_50px_rgba(23,33,43,0.28)]";
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
    viewer.className = "fixed inset-0 z-50 hidden items-start justify-center overflow-y-auto bg-[rgba(15,23,42,0.7)] p-3 sm:items-center sm:p-6";
    viewer.innerHTML = `
      <div class="relative my-3 flex max-h-[calc(100vh-1.5rem)] w-full max-w-6xl flex-col overflow-hidden rounded-[2rem] border border-[rgba(141,116,64,0.8)] bg-[rgba(36,48,61,0.97)] shadow-2xl sm:my-0 sm:max-h-[calc(100vh-3rem)]">
        <div class="flex flex-col gap-3 border-b border-[rgba(141,116,64,0.35)] px-4 py-4 text-[#f4ead0] sm:flex-row sm:flex-wrap sm:items-center sm:justify-between sm:px-5">
          <div>
            <p data-image-caption class="text-sm font-semibold tracking-[0.14em] text-[#eddca6]"></p>
            <p data-image-file class="mt-1 text-xs text-[rgba(244,234,208,0.72)]"></p>
          </div>
          <div class="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center">
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
        <div data-image-stage class="relative h-[60vh] overflow-hidden bg-[rgba(21,29,38,0.96)] sm:h-[78vh]">
          <img data-image-element class="absolute left-1/2 top-1/2 max-w-none select-none rounded-2xl bg-white shadow-2xl" alt="" draggable="false" />
        </div>
        <div class="flex flex-col gap-2 border-t border-[rgba(141,116,64,0.35)] px-4 py-3 text-xs text-[rgba(244,234,208,0.68)] sm:flex-row sm:flex-wrap sm:items-center sm:justify-between sm:px-5">
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
    const altText = sanitiseImageAltText(caption, fileName);
    image.setAttribute("alt", altText);
    text.textContent = altText;
    file.textContent = fileName || "";
    setImageViewerStatus("");
    viewer.classList.remove("hidden");
    viewer.classList.add("flex");

    if (image.complete) {
      resetImageViewerTransform();
    }
  }

  // sanitiseImageAltText returns a safe alt-text string for the image
  // preview modal. Captions pasted from another source may contain
  // HTML markup; raw markup must never reach the alt attribute because
  // screen readers may interpret it inconsistently. Mirrors the
  // imageAltText helper used in the templ SoldierCard so both
  // surfaces behave the same way. Audit issue #118.
  function sanitiseImageAltText(caption, fileName) {
    const stripped = sanitiseImageAltText.stripHtml(String(caption || ""));
    const cleaned = stripped.replace(/\s+/g, " ").trim();
    if (cleaned) {
      return cleaned;
    }
    const file = String(fileName || "").trim();
    if (file) {
      return file;
    }
    return "Archive image";
  }
  sanitiseImageAltText.stripHtml = function stripHtml(value) {
    if (!value || value.indexOf("<") === -1) {
      return value;
    }
    // Drop everything between < and > including the contents of
    // <script>, <style>, etc. Browser .textContent would keep the
    // inner text of those tags which is not what we want here.
    return value.replace(/<[^>]*>/g, "");
  };

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
      const params = new URLSearchParams();
      for (const [key, value] of data.entries()) {
        if (value instanceof File) continue;
        params.append(key, String(value));
      }
      const response = await fetch("/scratchpad/open", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
          "X-Requested-With": "DixieData"
        },
        body: params.toString(),
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

  function draftStorageKeyForForm(form) {
    const key = draftKeyForForm(form);
    return key ? `dixiedata:${key}` : "";
  }

  function draftKindForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "new";
    }
    return form.getAttribute("data-record-persistence-kind") || "new";
  }

  function draftRecordVersionForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    return form.getAttribute("data-draft-record-version") || "";
  }

  function draftResetPathForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    return form.getAttribute("data-draft-reset-path") || "";
  }

  function isDraftableField(field) {
    if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement || field instanceof HTMLSelectElement)) {
      return false;
    }
    if (!field.name || field.disabled) {
      return false;
    }
    if (field instanceof HTMLInputElement && (field.type === "file" || field.type === "hidden" || field.readOnly)) {
      return false;
    }
    return true;
  }

  function recordPersistenceTarget(form) {
    if (!(form instanceof HTMLFormElement)) {
      return null;
    }
    const target = form.querySelector("[data-record-persistence]");
    return target instanceof HTMLElement ? target : null;
  }

  function loadDeletedDraftState() {
    try {
      const raw = window.sessionStorage.getItem(deletedDraftStateStorageKey);
      if (!raw) {
        return null;
      }
      const parsed = JSON.parse(raw);
      return parsed && typeof parsed === "object" ? parsed : null;
    } catch (error) {
      return null;
    }
  }

  function saveDeletedDraftState(state) {
    try {
      if (!state) {
        window.sessionStorage.removeItem(deletedDraftStateStorageKey);
        return;
      }
      window.sessionStorage.setItem(deletedDraftStateStorageKey, JSON.stringify(state));
    } catch (error) {
    }
  }

  function clearDeletedDraftState() {
    try {
      window.sessionStorage.removeItem(deletedDraftStateStorageKey);
    } catch (error) {
    }
  }

  function deletedDraftStateForForm(form) {
    const state = loadDeletedDraftState();
    if (!state || state.draftKey !== draftKeyForForm(form)) {
      return null;
    }
    return state;
  }

  function clearDeletedDraftStateForForm(form) {
    if (deletedDraftStateForForm(form)) {
      clearDeletedDraftState();
    }
  }

  function formRecordRowCount(form) {
    if (!(form instanceof HTMLFormElement)) {
      return 1;
    }
    const count = form.querySelectorAll("[data-record-row]").length;
    return count > 0 ? count : 1;
  }

  function setRecordRowCount(form, targetCount) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const desired = Math.max(1, targetCount || 1);
    let rows = Array.from(form.querySelectorAll("[data-record-row]"));
    while (rows.length < desired) {
      const addButton = form.querySelector("[data-record-add]");
      if (!(addButton instanceof HTMLElement)) {
        break;
      }
      addRecordRow(addButton);
      rows = Array.from(form.querySelectorAll("[data-record-row]"));
    }
    while (rows.length > desired) {
      const row = rows.pop();
      if (!(row instanceof HTMLElement)) {
        break;
      }
      row.remove();
    }
  }

  function draftFieldValue(field) {
    if (field instanceof HTMLInputElement) {
      if (field.type === "checkbox") {
        return field.checked ? String(field.value || "1") : "";
      }
      if (field.type === "radio") {
        return field.checked ? String(field.value || "on") : "";
      }
    }
    return String(field.value ?? "");
  }

  function serializeDraftFields(form) {
    const payload = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!isDraftableField(field)) {
        return;
      }
      if (!Object.prototype.hasOwnProperty.call(payload, field.name)) {
        payload[field.name] = [];
      }
      payload[field.name].push(draftFieldValue(field));
    });
    return payload;
  }

  function cloneDraftSnapshot(snapshot) {
    const clone = {};
    Object.entries(snapshot || {}).forEach(([name, values]) => {
      clone[name] = Array.isArray(values) ? values.map((value) => String(value ?? "")) : [];
    });
    return clone;
  }

  function snapshotsEqual(left, right) {
    const names = new Set([].concat(Object.keys(left || {}), Object.keys(right || {})));
    for (const name of names) {
      const leftValues = Array.isArray(left?.[name]) ? left[name] : [];
      const rightValues = Array.isArray(right?.[name]) ? right[name] : [];
      if (leftValues.length !== rightValues.length) {
        return false;
      }
      for (let index = 0; index < leftValues.length; index += 1) {
        if (String(leftValues[index] ?? "") !== String(rightValues[index] ?? "")) {
          return false;
        }
      }
    }
    return true;
  }

  function mergeDraftSnapshot(base, overrides) {
    const merged = cloneDraftSnapshot(base || {});
    Object.entries(overrides || {}).forEach(([name, values]) => {
      merged[name] = Array.isArray(values) ? values.map((value) => String(value ?? "")) : [];
    });
    return merged;
  }

  function baselineStateForForm(form) {
    let state = draftBaselines.get(form);
    if (state) {
      return state;
    }
    state = {
      fields: serializeDraftFields(form),
      rowCount: formRecordRowCount(form),
      version: draftRecordVersionForForm(form),
    };
    draftBaselines.set(form, state);
    return state;
  }

  function normalizeDraftSnapshot(raw) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
      return {};
    }
    const normalized = {};
    Object.entries(raw).forEach(([name, values]) => {
      if (!Array.isArray(values)) {
        return;
      }
      normalized[name] = values.map((value) => String(value ?? ""));
    });
    return normalized;
  }

  function calculateDraftRowCount(snapshot) {
    return Math.max(
      1,
      Array.isArray(snapshot?.record_type) ? snapshot.record_type.length : 0,
      Array.isArray(snapshot?.record_app_id) ? snapshot.record_app_id.length : 0,
      Array.isArray(snapshot?.record_details) ? snapshot.record_details.length : 0,
    );
  }

  function buildDraftPayload(form) {
    const kind = draftKindForForm(form);
    const currentFields = serializeDraftFields(form);
    if (kind === "edit") {
      const baseline = baselineStateForForm(form);
      const delta = {};
      let changed = false;
      const names = new Set([].concat(Object.keys(baseline.fields || {}), Object.keys(currentFields || {})));
      names.forEach((name) => {
        const baselineValues = Array.isArray(baseline.fields?.[name]) ? baseline.fields[name] : [];
        const currentValues = Array.isArray(currentFields?.[name]) ? currentFields[name] : [];
        if (baselineValues.length !== currentValues.length || currentValues.some((value, index) => String(value ?? "") !== String(baselineValues[index] ?? ""))) {
          delta[name] = currentValues.map((value) => String(value ?? ""));
          changed = true;
        }
      });
      if (!changed) {
        return null;
      }
      return {
        schema: 2,
        kind: "edit",
        version: baseline.version,
        rowCount: formRecordRowCount(form),
        fields: delta,
      };
    }
    return {
      schema: 2,
      kind: "new",
      rowCount: formRecordRowCount(form),
      fields: currentFields,
    };
  }

  function persistDraftForForm(form) {
    const storageKey = draftStorageKeyForForm(form);
    if (!storageKey) {
      return { hasDraft: false };
    }
    staleDrafts.delete(form);
    const payload = buildDraftPayload(form);
    try {
      if (!payload) {
        window.localStorage.removeItem(storageKey);
        return { hasDraft: false };
      }
      clearDeletedDraftStateForForm(form);
      window.localStorage.setItem(storageKey, JSON.stringify(payload));
      return { hasDraft: true };
    } catch (error) {
      return { hasDraft: false };
    }
  }

  function previewValueDisplay(field, rawValue) {
    const normalized = String(rawValue ?? "");
    if (field instanceof HTMLInputElement && field.type === "checkbox") {
      return normalized === "" ? "No" : "Yes";
    }
    if (field instanceof HTMLSelectElement) {
      const option = Array.from(field.options).find((candidate) => candidate.value === normalized);
      if (option) {
        return option.textContent?.trim() || normalized || "(blank)";
      }
    }
    return normalized.trim() === "" ? "(blank)" : normalized;
  }

  function draftFieldLabel(name, occurrence) {
    const labels = {
      display_id: "Display ID",
      entry_type: "Entry Type",
      spouse_soldier_id: "Linked Soldier",
      relationship_label: "Relationship Label",
      maiden_name: "Maiden Name",
      pension_id: "Pension ID",
      application_id: "Application ID",
      prefix: "Prefix",
      show_prefix_before_name: "Show prefix before name",
      first_name: "First Name",
      middle_name: "Middle Name",
      last_name: "Last Name",
      suffix: "Suffix",
      rank: "Rank",
      rank_in: "Rank In",
      rank_out: "Rank Out",
      unit: "Unit",
      pension_state: "Pension State",
      confederate_home_status: "Confederate Home Status",
      confederate_home_name: "Confederate Home Name",
      death_year: "Death Year",
      death_month: "Death Month",
      death_day: "Death Day",
      birth_date: "Birth Date",
      death_date: "Death Date",
      birth_info: "Birth Info",
      buried_in: "Buried In",
      biography: "Biography",
      pdf_excerpt_override: "Advanced PDF Excerpt Override",
      notes: "Internal Notes",
      record_type: "Source Record Type",
      record_app_id: "Source Record Number",
      record_details: "Source Record Details",
    };
    const base = labels[name] || name.replace(/_/g, " ").replace(/\b\w/g, (value) => value.toUpperCase());
    if (name === "record_type" || name === "record_app_id" || name === "record_details") {
      return `Source Record ${occurrence + 1} - ${base}`;
    }
    return base;
  }

  function buildDraftDiffEntries(form, baselineFields, draftSnapshot) {
    const entries = [];
    const names = Array.from(new Set([].concat(Object.keys(baselineFields || {}), Object.keys(draftSnapshot || {})))).sort();
    names.forEach((name) => {
      const field = form.querySelector(`[name="${name}"]`);
      const baselineValues = Array.isArray(baselineFields?.[name]) ? baselineFields[name] : [];
      const draftValues = Array.isArray(draftSnapshot?.[name]) ? draftSnapshot[name] : [];
      const count = Math.max(baselineValues.length, draftValues.length, 1);
      for (let index = 0; index < count; index += 1) {
        const currentValue = String(baselineValues[index] ?? "");
        const localValue = String(draftValues[index] ?? "");
        if (currentValue === localValue) {
          continue;
        }
        entries.push({
          label: draftFieldLabel(name, index),
          currentValue: previewValueDisplay(field, currentValue),
          localValue: previewValueDisplay(field, localValue),
        });
      }
    });
    return entries;
  }

  function renderRecordPersistencePreview(form, entries, showReapply) {
    const target = recordPersistenceTarget(form);
    if (!(target instanceof HTMLElement)) {
      return;
    }
    const preview = target.querySelector("[data-record-persistence-preview]");
    const previewTitle = target.querySelector("[data-record-persistence-preview-title]");
    const previewMessage = target.querySelector("[data-record-persistence-preview-message]");
    const previewList = target.querySelector("[data-record-persistence-preview-list]");
    const reapply = target.querySelector("[data-reapply-stale-draft]");
    if (!(preview instanceof HTMLElement) || !(previewTitle instanceof HTMLElement) || !(previewMessage instanceof HTMLElement) || !(previewList instanceof HTMLElement) || !(reapply instanceof HTMLElement)) {
      return;
    }
    if (!Array.isArray(entries) || entries.length === 0) {
      preview.classList.add("hidden");
      previewTitle.textContent = "Review older saved local changes";
      previewMessage.textContent = "";
      previewList.innerHTML = "";
      reapply.textContent = "Reapply older saved local changes";
      reapply.classList.add("hidden");
      return;
    }
    preview.classList.remove("hidden");
    previewTitle.textContent = "Review older saved local changes";
    previewMessage.textContent = "Current form values are coming from the database. Reapplying will replace only the fields listed below with the saved local draft values.";
    previewList.innerHTML = "";
    entries.forEach((entry) => {
      const item = document.createElement("li");
      item.className = "rounded-xl border border-amber-700/20 bg-amber-50/50 px-3 py-2";
      const label = document.createElement("p");
      label.className = "font-semibold text-amber-950";
      label.textContent = entry.label;
      const dbValue = document.createElement("p");
      dbValue.className = "mt-1 text-xs text-amber-900";
      dbValue.textContent = `Database value: ${entry.currentValue}`;
      const localValue = document.createElement("p");
      localValue.className = "mt-1 text-xs text-amber-900";
      localValue.textContent = `Saved local draft value: ${entry.localValue}`;
      item.appendChild(label);
      item.appendChild(dbValue);
      item.appendChild(localValue);
      previewList.appendChild(item);
    });
    reapply.textContent = "Reapply older saved local changes";
    reapply.classList.toggle("hidden", !showReapply);
  }

  function hideDraftDeleteConfirmation(form, scope, options = {}) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const normalized = scope === "stale" ? "stale" : "base";
    const trigger = form.querySelector(`[data-clear-draft-trigger="${normalized}"]`);
    const confirm = form.querySelector(`[data-clear-draft-confirm="${normalized}"]`);
    if (trigger instanceof HTMLElement && options.restoreTrigger !== false) {
      trigger.classList.remove("hidden");
    }
    if (confirm instanceof HTMLElement) {
      confirm.classList.add("hidden");
    }
    if ((form.dataset.clearDraftConfirmScope || "") === normalized) {
      delete form.dataset.clearDraftConfirmScope;
    }
  }

  function showDraftDeleteConfirmation(form, scope) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const normalized = scope === "stale" ? "stale" : "base";
    const trigger = form.querySelector(`[data-clear-draft-trigger="${normalized}"]`);
    const confirm = form.querySelector(`[data-clear-draft-confirm="${normalized}"]`);
    if (!(trigger instanceof HTMLElement) || !(confirm instanceof HTMLElement)) {
      return;
    }
    form.dataset.clearDraftConfirmScope = normalized;
    trigger.classList.add("hidden");
    confirm.classList.remove("hidden");
    const cancel = confirm.querySelector(`[data-cancel-clear-draft="${normalized}"]`);
    if (cancel instanceof HTMLButtonElement) {
      cancel.focus();
    }
  }

  function resetDraftDeleteConfirmations(form) {
    hideDraftDeleteConfirmation(form, "base", { restoreTrigger: false });
    hideDraftDeleteConfirmation(form, "stale", { restoreTrigger: false });
  }

  function syncDraftDeleteControls(form, state) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const baseTrigger = form.querySelector('[data-clear-draft-trigger="base"]');
    const staleTrigger = form.querySelector('[data-clear-draft-trigger="stale"]');
    const staleReapply = form.querySelector("[data-reapply-stale-draft]");
    if (baseTrigger instanceof HTMLElement) {
      baseTrigger.classList.add("hidden");
    }
    if (staleTrigger instanceof HTMLElement) {
      staleTrigger.classList.add("hidden");
    }
    if (staleReapply instanceof HTMLElement) {
      staleReapply.classList.add("hidden");
    }
    resetDraftDeleteConfirmations(form);
    if (state === "stale") {
      if (staleTrigger instanceof HTMLElement) {
        staleTrigger.classList.remove("hidden");
      }
      if (staleReapply instanceof HTMLElement) {
        staleReapply.classList.remove("hidden");
      }
      return;
    }
    if (state === "dirty" || state === "restored") {
      if (baseTrigger instanceof HTMLElement) {
        baseTrigger.classList.remove("hidden");
      }
    }
  }

  function syncDeletedDraftUndo(form, visible) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const panel = form.querySelector("[data-cleared-draft-undo]");
    if (!(panel instanceof HTMLElement)) {
      return;
    }
    panel.classList.toggle("hidden", !visible);
  }

  function setRecordPersistenceState(form, state, options = {}) {
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
    form.dataset.recordPersistenceState = state;
    renderRecordPersistencePreview(form, [], false);
    if (kind === "edit" && state === "clean") {
      heading = "Committed to database.";
      message = "This person record currently matches the primary database until you make new local edits.";
      target.classList.add("border-emerald-700/40", "bg-emerald-50/80", "text-emerald-900");
    } else if (kind === "edit" && state === "restored") {
      heading = "Local draft restored.";
      message = "Your current changes are cached in localStorage and have not been committed to the database yet.";
      target.classList.add("border-amber-700/40", "bg-amber-50/80", "text-amber-900");
    } else if (kind === "edit" && state === "stale") {
      heading = "Older saved local draft not applied.";
      message = "This form is showing the current database values because the saved local draft is older than the database record.";
      target.classList.add("border-amber-700/40", "bg-amber-50/80", "text-amber-900");
      renderRecordPersistencePreview(form, options.entries || [], true);
    } else {
      heading = kind === "edit" ? "Unsaved local edits." : (state === "restored" ? "Local draft restored." : "Local draft only.");
      message = kind === "edit"
        ? "Your current changes are cached in localStorage and have not been committed to the database yet."
        : "This new person record is cached in localStorage until you create it in the database.";
      target.classList.add("border-amber-700/40", "bg-amber-50/80", "text-amber-900");
    }
    headingNode.textContent = heading;
    messageNode.textContent = message;
    syncDraftDeleteControls(form, state);
    syncDeletedDraftUndo(form, Boolean(options.showUndo));
  }

  function clearDraftForForm(form, options = {}) {
    const storageKey = draftStorageKeyForForm(form);
    if (!storageKey) {
      return;
    }
    let savedDraft = null;
    try {
      savedDraft = window.localStorage.getItem(storageKey);
      window.localStorage.removeItem(storageKey);
    } catch (error) {
    }
    if (options.rememberDeleted && savedDraft) {
      saveDeletedDraftState({
        draftKey: draftKeyForForm(form),
        payload: savedDraft,
      });
    } else if (!options.preserveDeletedState) {
      clearDeletedDraftStateForForm(form);
    }
    staleDrafts.delete(form);
    resetDraftDeleteConfirmations(form);
    setRecordPersistenceState(form, draftKindForForm(form) === "edit" ? "clean" : "dirty", { showUndo: Boolean(options.rememberDeleted && savedDraft) });
  }

  function confirmDeleteDraftFromControl(control) {
    const form = ownerForm(control);
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    clearDraftForForm(form, { rememberDeleted: true });
    const resetPath = draftResetPathForForm(form);
    if (resetPath) {
      window.location.assign(resetPath);
    }
  }

  function applyDraftSnapshot(form, snapshot, rowCount) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    setRecordRowCount(form, rowCount || calculateDraftRowCount(snapshot));
    const cursors = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!isDraftableField(field)) {
        return;
      }
      const values = Array.isArray(snapshot?.[field.name]) ? snapshot[field.name] : [];
      const index = cursors[field.name] || 0;
      const rawValue = index < values.length ? String(values[index] ?? "") : "";
      if (field instanceof HTMLInputElement && field.type === "checkbox") {
        field.checked = rawValue !== "";
      } else if (field instanceof HTMLInputElement && field.type === "radio") {
        field.checked = rawValue !== "" && field.value === rawValue;
      } else {
        field.value = rawValue;
      }
      cursors[field.name] = index + 1;
    });
    syncEntryTypeFields(form);
    syncConfederateHomeFields(form);
    form.querySelectorAll("[data-live-count-target]").forEach((field) => {
      if (field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement) {
        updateLiveCount(field);
      }
    });
  }

  function readStoredDraft(form) {
    const storageKey = draftStorageKeyForForm(form);
    if (!storageKey) {
      return null;
    }
    let saved;
    try {
      saved = window.localStorage.getItem(storageKey);
    } catch (error) {
      return null;
    }
    if (!saved) {
      return null;
    }
    let payload;
    try {
      payload = JSON.parse(saved);
    } catch (error) {
      return null;
    }
    if (payload && payload.schema === 2 && payload.fields && typeof payload.fields === "object") {
      return {
        schema: 2,
        kind: payload.kind === "edit" ? "edit" : "new",
        version: String(payload.version || ""),
        rowCount: Number.parseInt(String(payload.rowCount || ""), 10) || calculateDraftRowCount(payload.fields),
        fields: normalizeDraftSnapshot(payload.fields),
        legacy: false,
      };
    }
    return {
      schema: 1,
      kind: draftKindForForm(form),
      version: "",
      rowCount: calculateDraftRowCount(payload),
      fields: normalizeDraftSnapshot(payload),
      legacy: true,
    };
  }

  function restoreDraftForForm(form) {
    const baseline = baselineStateForForm(form);
    const stored = readStoredDraft(form);
    if (!stored) {
      setRecordPersistenceState(form, draftKindForForm(form) === "edit" ? "clean" : "dirty", { showUndo: Boolean(deletedDraftStateForForm(form)) });
      return;
    }
    clearDeletedDraftStateForForm(form);
    const effectiveSnapshot = stored.kind === "edit" ? mergeDraftSnapshot(baseline.fields, stored.fields) : cloneDraftSnapshot(stored.fields);
    if (stored.kind === "edit" && snapshotsEqual(baseline.fields, effectiveSnapshot)) {
      clearDraftForForm(form);
      return;
    }
    if (stored.kind === "edit" && (stored.legacy || stored.version !== baseline.version)) {
      const entries = buildDraftDiffEntries(form, baseline.fields, effectiveSnapshot);
      staleDrafts.set(form, {
        snapshot: effectiveSnapshot,
        rowCount: stored.rowCount || baseline.rowCount,
        entries,
      });
      setRecordPersistenceState(form, "stale", { entries });
      return;
    }
    applyDraftSnapshot(form, effectiveSnapshot, stored.rowCount);
    setRecordPersistenceState(form, "restored");
  }

  function reapplyStaleDraftFromControl(control) {
    const form = ownerForm(control);
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const stale = staleDrafts.get(form);
    if (!stale) {
      return;
    }
    applyDraftSnapshot(form, stale.snapshot, stale.rowCount);
    staleDrafts.delete(form);
    const result = persistDraftForForm(form);
    setRecordPersistenceState(form, result.hasDraft ? "dirty" : "clean");
  }

  function undoDeletedDraftFromControl(control) {
    const form = ownerForm(control);
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const deletedState = deletedDraftStateForForm(form);
    const storageKey = draftStorageKeyForForm(form);
    if (!deletedState || !storageKey || typeof deletedState.payload !== "string" || deletedState.payload.length === 0) {
      syncDeletedDraftUndo(form, false);
      return;
    }
    try {
      window.localStorage.setItem(storageKey, deletedState.payload);
      clearDeletedDraftState();
      restoreDraftForForm(form);
      showToast("Saved local draft restored.", "success");
    } catch (error) {
    }
  }

  function initializeDraftForms() {
    document.querySelectorAll("form[data-draft-key]").forEach((form) => {
      if (form instanceof HTMLFormElement) {
        draftBaselines.set(form, {
          fields: serializeDraftFields(form),
          rowCount: formRecordRowCount(form),
          version: draftRecordVersionForForm(form),
        });
        restoreDraftForForm(form);
      }
    });
  }

  // Floating nav toggle: the click handler is bound inline in
  // layout.templ (onclick="…toggle('hidden')") so it works even
  // before this script runs. This init function only owns the
  // outside-click-to-close behavior, which is safe to defer until
  // after DOMContentLoaded.
  function initializeFloatingNav() {
    const panel = document.querySelector("[data-floating-nav-panel]");
    const toggle = document.querySelector("[data-floating-nav-toggle]");
    if (!(panel instanceof HTMLElement) || !(toggle instanceof HTMLButtonElement)) {
      return;
    }
    document.addEventListener("click", (event) => {
      if (panel.classList.contains("hidden")) {
        return;
      }
      if (event.target instanceof Node && !panel.contains(event.target) && !toggle.contains(event.target)) {
        panel.classList.add("hidden");
      }
    });
  }

  // Browse filter drawer. Counts active filters and updates the badge
  // above the disclosure element. Persists open/closed preference in
  // localStorage so the drawer stays collapsed/expanded across visits.
  function initializeBrowseFilterDrawer() {
    const form = document.querySelector("[data-browse-filters-form]");
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const details = form.querySelector("[data-browse-filters-details]");
    const countNode = form.querySelector("[data-browse-filters-count]");
    if (!(details instanceof HTMLDetailsElement) || !(countNode instanceof HTMLElement)) {
      return;
    }

    const storageKey = "dixiedata.browse.filters.open";

    // Restore open/closed preference.
    try {
      const stored = window.localStorage.getItem(storageKey);
      if (stored === "true") {
        details.open = true;
      } else if (stored === "false") {
        details.open = false;
      }
    } catch (error) {
      // Ignore storage errors.
    }

    // Count of filters that differ from the default state. The default for
    // each input is recorded the first time we see it; subsequent counts
    // compare against that baseline so users who pick the default option
    // for a non-empty default (e.g. sort) don't get a phantom badge.
    function updateCount() {
      const inputs = form.querySelectorAll("[data-browse-filter-input]");
      let active = 0;
      inputs.forEach((input) => {
        if (!(input instanceof HTMLInputElement || input instanceof HTMLSelectElement)) {
          return;
        }
        const value = (input.value || "").trim();
        // A filter is "active" if it's non-empty AND not the default scope/sort/page_size.
        if (!value) {
          return;
        }
        const name = input.getAttribute("name");
        if (name === "scope" && value === "all") return;
        if (name === "sort" && value === "display_id_asc") return;
        if (name === "page_size" && value === "100") return;
        active += 1;
      });
      countNode.textContent = active === 0 ? "0 active" : `${active} active`;
    }

    updateCount();

    // Update count when any filter changes.
    form.addEventListener("change", () => updateCount());
    form.addEventListener("input", () => updateCount());

    // Persist open/closed preference when toggled.
    details.addEventListener("toggle", () => {
      try {
        window.localStorage.setItem(storageKey, details.open ? "true" : "false");
      } catch (error) {
        // Ignore storage errors.
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
      // Show the pension fields (state, id, application id) for any
      // entry type that can file for a pension: soldier, wife, or
      // widow. Issue #75: pension_state was previously hidden for
      // wife entry type because the JS read `widowEntry` only; wives
      // can also file for their husband's pension while he's alive
      // (in case he becomes disabled) or for widow's pension later.
      // linked_person stays hidden — the role is non-pensioner.
      setSectionEnabled(section, isSoldierEntryType(select.value) || spouseEntry);
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

  function updateLiveCount(input) {
    if (!(input instanceof HTMLTextAreaElement || input instanceof HTMLInputElement)) {
      return;
    }
    const target = input.getAttribute("data-live-count-target");
    if (!target) {
      return;
    }
    const count = input.value.length;
    document.querySelectorAll(`[data-live-count-display="${target}"]`).forEach((node) => {
      const budgetRaw = input.getAttribute("data-live-count-budget");
      const budget = Number.parseInt(String(budgetRaw || ""), 10);
      node.textContent = Number.isInteger(budget) && budget > 0 ? `${count} / ${budget}` : `${count} chars`;
    });
    document.querySelectorAll(`[data-live-count-status="${target}"]`).forEach((node) => {
      const budgetRaw = input.getAttribute("data-live-count-budget");
      const budget = Number.parseInt(String(budgetRaw || ""), 10);
      if (!Number.isInteger(budget) || budget <= 0) {
        node.textContent = "";
        node.classList.remove("text-[#6f2c26]", "text-emerald-700");
        node.classList.add("text-slate-500");
        return;
      }
      if (count > budget) {
        node.textContent = `Over export-fit target by ${count - budget} chars.`;
        node.classList.remove("text-slate-500", "text-emerald-700");
        node.classList.add("text-[#6f2c26]");
        return;
      }
      node.textContent = `Within export-fit target. ${budget - count} chars remaining.`;
      node.classList.remove("text-slate-500", "text-[#6f2c26]");
      node.classList.add("text-emerald-700");
    });
  }

  function initializeLiveCounts(root = document) {
    root.querySelectorAll("[data-live-count-input]").forEach((input) => {
      if (!(input instanceof HTMLTextAreaElement || input instanceof HTMLInputElement)) {
        return;
      }
      input.removeEventListener("input", input.__dixieLiveCountHandler || (() => {}));
      const handler = () => updateLiveCount(input);
      input.__dixieLiveCountHandler = handler;
      input.addEventListener("input", handler);
      updateLiveCount(input);
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
    const headers = {
      success: "Success",
      info: "Heads up",
      warning: "Warning",
      error: "Attention",
    };
    const toast = document.createElement("div");
    toast.className = "toast-card";
    toast.setAttribute("data-toast-kind", kind);
    toast.innerHTML = `
      <div>
        <div class="text-xs font-semibold uppercase tracking-[0.22em] text-[#8d7440]">${headers[kind] || "Notice"}</div>
        <div class="mt-1 text-sm text-[#22303d]">${message}</div>
      </div>
      <button type="button" class="secondary-button px-3 py-1 text-xs" data-toast-dismiss>Dismiss</button>
    `;
    region.appendChild(toast);
    const dismiss = () => {
      toast.setAttribute("data-toast-dismissing", "true");
      window.setTimeout(() => toast.remove(), toastFadeOutMs);
    };
    toast.querySelector("[data-toast-dismiss]")?.addEventListener("click", dismiss);
    // Auto-dismiss success/info after toastAutoDismissMs. Error
    // and warning stay until the user dismisses them —
    // preserves the manual-dismiss decision from Issue #54.
    if (kind === "success" || kind === "info") {
      window.setTimeout(dismiss, toastAutoDismissMs);
    }
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

  // After a response populates the shared status panel, bring it into
  // view so the user sees the result without scrolling. Without this,
  // a successful import can land in a panel y=1372 below a viewport
  // fold at 800px — user clicks Load Backup, nothing visibly happens,
  // re-imports. Only scrolls when the panel content actually changed
  // (the placeholder "Import + export status messages appear here."
  // text is not a result; ignore that re-render).
  function scrollShareStatusIntoView(target) {
    if (!(target instanceof HTMLElement) || target.id !== "share-status") {
      return;
    }
    const placeholder = "Import + export status messages appear here.";
    if (target.textContent && target.textContent.includes(placeholder)) {
      return;
    }
    try {
      target.scrollIntoView({ behavior: "smooth", block: "nearest" });
    } catch (error) {
      console.warn("scrollShareStatusIntoView failed", error);
    }
  }

  // htmx dispatches its own swaps (XHR, not fetch). Hook htmx:afterSwap
  // so the shared status panel scrolls into view after a response lands
  // in it, regardless of whether the swap came from app.js's
  // applyResponse or htmx's internal ajax.
  if (typeof window !== "undefined" && window.htmx && typeof window.htmx.on === "function") {
    window.htmx.on("htmx:afterSwap", (event) => {
      scrollShareStatusIntoView(event.target);
    });
  }

  async function refreshCalendarGrid(monthValue) {
    const month = Number.parseInt(String(monthValue || ""), 10);
    if (!Number.isInteger(month) || month < 1 || month > 12) {
      return;
    }
    const target = document.getElementById("calendar-grid-panel");
    if (!(target instanceof HTMLElement)) {
      return;
    }
    try {
      const response = await fetch(`/calendar/${month}/grid`, {
        headers: {
          "X-Requested-With": "DixieData"
        }
      });
      if (!response.ok) {
        return;
      }
      target.outerHTML = await response.text();
      initializeDynamicContent();
    } catch (error) {
      // Leave the current grid in place if refresh fails.
    }
  }

  function initializeDynamicContent() {
    applyResponsiveLayout(document);
    initializeTabs();
    initializeEntryTypeForms();
    initializeLiveCounts(document);
    restoreRedirectState();
    restorePendingToast();
    applySmartBackLabels();
    rememberRecentRecordFromPage();
    hydrateRecentSearchResults();
    initializeBrowseView();
    applyCalendarAnniversaryDensity();
    openPrintConfigFromQuery();
    initializeCopyPathButtons();
    document.querySelectorAll("form[data-pdf-pref-scope]").forEach((form) => applyPDFPreferences(form));
  }

  // initializeCopyPathButtons binds click handlers to every
  // [data-copy-path] button. The button stores the absolute file
  // path in its data-copy-path attribute; clicking copies the path
  // to the clipboard and shows a brief "Path copied" toast. Used on
  // /jobs/{id} completion state and the report view so the user can
  // find the saved artifact in either runtime (Wails desktop or
  // web-mode) without depending on the OS shell to open it.
  function initializeCopyPathButtons() {
    document.querySelectorAll("[data-copy-path]").forEach((button) => {
      if (button.__copyPathBound) {
        return;
      }
      button.__copyPathBound = true;
      button.addEventListener("click", async () => {
        const path = button.getAttribute("data-copy-path") || "";
        if (!path) {
          showToast("No path to copy.", "error");
          return;
        }
        try {
          if (navigator.clipboard?.writeText) {
            await navigator.clipboard.writeText(path);
          } else {
            // Fallback for browsers without the async clipboard API.
            const tmp = document.createElement("textarea");
            tmp.value = path;
            tmp.style.position = "fixed";
            tmp.style.opacity = "0";
            document.body.appendChild(tmp);
            tmp.select();
            document.execCommand("copy");
            document.body.removeChild(tmp);
          }
          showToast("Path copied.", "success");
        } catch (error) {
          showToast("Could not copy the path. Long-press to select.", "error");
        }
      });
    });
  }

  function currentBrowseStateFromForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return {};
    }
    const data = new FormData(form);
    return {
      page: data.get("page") || "1",
      page_size: data.get("page_size") || "100",
      scope: data.get("scope") || "all",
      sort: data.get("sort") || "display_id_asc",
      entry_type: data.get("entry_type") || "",
      unit: data.get("unit") || "",
      buried_in: data.get("buried_in") || "",
      pension_state: data.get("pension_state") || "",
      review_status: data.get("review_status") || "",
      confederate_home_status: data.get("confederate_home_status") || "",
    };
  }

  function applyBrowseStateToForm(form, state) {
    if (!(form instanceof HTMLFormElement) || !state || typeof state !== "object") {
      return;
    }
    ["page", "page_size", "scope", "sort", "entry_type", "unit", "buried_in", "pension_state", "review_status", "confederate_home_status"].forEach((name) => {
      const field = form.elements.namedItem(name);
      if (field instanceof HTMLInputElement || field instanceof HTMLSelectElement) {
        if (typeof state[name] === "string") {
          field.value = state[name];
        }
      }
    });
  }

  function browseStateDiffers(current, saved) {
    return ["page", "page_size", "scope", "sort", "entry_type", "unit", "buried_in", "pension_state", "review_status", "confederate_home_status"]
      .some((key) => String(current[key] || "") !== String(saved[key] || ""));
  }

  function applyBrowseColumns(root) {
    const enabled = new Set(loadBrowseColumns());
    root.querySelectorAll("[data-browse-column-toggle]").forEach((toggle) => {
      if (toggle instanceof HTMLInputElement) {
        toggle.checked = enabled.has(toggle.value);
      }
    });
    root.querySelectorAll("[data-browse-column]").forEach((cell) => {
      if (cell instanceof HTMLElement) {
        const key = cell.getAttribute("data-browse-column") || "";
        cell.classList.toggle("hidden", !enabled.has(key));
      }
    });
  }

  function updateBrowseSelectionStatus(root = document) {
    const selected = loadBrowseSelection();
    root.querySelectorAll("[data-browse-selection-status]").forEach((node) => {
      if (node instanceof HTMLElement) {
        node.textContent = selected.length > 0
          ? `${selected.length} record(s) selected across Browse pages and filters.`
          : "Select records across pages to keep a working set while you browse.";
      }
    });
  }

  function applyBrowseSelection(root) {
    const selected = new Set(loadBrowseSelection());
    root.querySelectorAll("[data-browse-select]").forEach((input) => {
      if (input instanceof HTMLInputElement) {
        const id = Number.parseInt(input.value || "", 10);
        input.checked = Number.isInteger(id) && selected.has(id);
      }
    });
    updateBrowseSelectionStatus(root);
  }

  function applyCalendarAnniversaryDensity(root = document) {
    const mode = loadCalendarAnniversaryDensity();
    const activeClasses = ["border-[#22303d]", "bg-[rgba(36,48,61,0.92)]", "text-[#f2ede1]"];
    const inactiveClasses = ["border-[rgba(141,116,64,0.24)]", "bg-white/70", "text-[#51606e]"];
    root.querySelectorAll("[data-calendar-anniversary-density]").forEach((container) => {
      if (!(container instanceof HTMLElement)) {
        return;
      }
      container.setAttribute("data-calendar-anniversary-density", mode);
      container.querySelectorAll("[data-calendar-anniversary-expanded]").forEach((row) => {
        if (row instanceof HTMLElement) {
          row.classList.toggle("hidden", mode !== "expanded");
        }
      });
      container.querySelectorAll("[data-calendar-anniversary-compact]").forEach((row) => {
        if (row instanceof HTMLElement) {
          row.classList.toggle("hidden", mode !== "compact");
        }
      });
    });
    root.querySelectorAll("[data-calendar-anniversary-density-toggle]").forEach((button) => {
      if (!(button instanceof HTMLButtonElement)) {
        return;
      }
      const isActive = button.getAttribute("data-calendar-anniversary-density-toggle") === mode;
      button.setAttribute("aria-pressed", isActive ? "true" : "false");
      activeClasses.forEach((className) => button.classList.toggle(className, isActive));
      inactiveClasses.forEach((className) => button.classList.toggle(className, !isActive));
    });
  }

  function initializeBrowseView() {
    const page = document.querySelector("[data-browse-page]");
    if (!(page instanceof HTMLElement)) {
      return;
    }
    const form = document.getElementById("browse-filters");
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    applyBrowseColumns(document);
    applyBrowseSelection(document);
    const current = currentBrowseStateFromForm(form);
    const saved = loadBrowseState();
    if (!page.hasAttribute("data-browse-restored") && browseStateDiffers(current, saved)) {
      page.setAttribute("data-browse-restored", "true");
      applyBrowseStateToForm(form, saved);
      const pageField = form.querySelector("[data-browse-page-input]");
      if (pageField instanceof HTMLInputElement || pageField instanceof HTMLSelectElement) {
        pageField.value = "1";
      }
      saveBrowseState(currentBrowseStateFromForm(form));
      return;
    }
    page.setAttribute("data-browse-restored", "true");
    saveBrowseState(currentBrowseStateFromForm(form));
  }

  function setBusyGroupState(el, busy) {
    if (!(el instanceof HTMLElement)) {
      return;
    }
    const group = (el.getAttribute("data-busy-group") || "").trim();
    if (!group) {
      return;
    }
    const selector = `[data-busy-group="${group}"]`;
    document.querySelectorAll(selector).forEach((member) => {
      if (!(member instanceof HTMLElement)) {
        return;
      }
      if (member === el) {
        return;
      }
      setBusyState(member, busy);
    });
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

  async function dispatchDixieDataForm(button) {
    // Bare-button mode: some share-page buttons have hx-post /
    // hx-delete but no enclosing <form>. Construct a synthetic form
    // from the button's hx-* URL so the fetch+redirect logic stays
    // uniform. This branch is dead code after the templ retag
    // (Commits 6–14) — every click target will live inside a form
    // with data-dixie-submit and a real action.
    const form = button instanceof HTMLFormElement
      ? button
      : button.closest("form") || (() => {
        // Bare-button mode: pull URL from data-action (Option C
        // convention) and method from data-method. After the templ
        // retag, no element carries hx-* / data-hx-* anymore.
        const url = (button.getAttribute && button.getAttribute("data-action")) || "";
        if (!url) return null;
        const method = button.getAttribute("data-method") === "DELETE" ? "DELETE" : "POST";
        const synthetic = document.createElement("form");
        synthetic.action = url;
        synthetic.method = method;
        return synthetic;
      })();
    if (!(form instanceof HTMLFormElement)) {
      return false;
    }
    const submitter = button instanceof HTMLElement ? button : null;
    const confirmMessage = (submitter && submitter.dataset && submitter.dataset.confirm)
      || (form.dataset && form.dataset.confirm);
    if (confirmMessage && !window.confirm(confirmMessage)) {
      return false;
    }
    setBusyState(submitter || form, true);
    setBusyGroupState(submitter || form, true);
    try {
      const explicitMethod = (form.getAttribute && form.getAttribute("method")) || "";
      const fetchOptions = { method: explicitMethod ? form.method.toUpperCase() : "POST" };
      // Only attach a body for non-GET / non-HEAD requests. Bare-button
      // synthetic forms have no FormData to attach anyway.
      const methodUpper = String(fetchOptions.method).toUpperCase();
      if (methodUpper !== "GET" && methodUpper !== "HEAD") {
        fetchOptions.body = button.closest("form")
          ? new FormData(form)
          : new FormData();
      }
      const requestUrl = form.action || window.location.pathname;
      const response = await fetch(requestUrl, fetchOptions);
      // Two response shapes to handle during the Option C migration:
      //   - 200 + X-DixieData-Redirect (new contract, after Commits 3–5)
      //   - 303 + Location (legacy contract, still active until Commits 3–5)
      // fetch() follows 303 by default; the response we see has status=200,
      // type="basic", and url pointing to the followed target. Detect via
      // response.url !== requestUrl (the legacy dispatcher did this too).
      const followedRedirect = response.redirected === true && response.url && response.url !== requestUrl;
      const dixieRedirect = !followedRedirect ? response.headers.get("X-DixieData-Redirect") : null;
      const legacyLocation = followedRedirect
        ? new URL(response.url).pathname + new URL(response.url).search
        : null;
      const redirectTo = dixieRedirect || legacyLocation;
      const toastMessage = followedRedirect ? null : response.headers.get("X-DixieData-Toast");
      const toastKind = followedRedirect ? "success" : (response.headers.get("X-DixieData-Toast-Type") || "success");
      const closeFeedback = followedRedirect ? null : response.headers.get("X-DixieData-Close-Feedback");
      const refreshCalendarMonth = followedRedirect ? null : response.headers.get("X-DixieData-Refresh-Calendar-Month");
      const responseOk = followedRedirect || response.ok;
      if (!responseOk && !redirectTo) {
        showToast(toastMessage || "Request failed.", toastKind === "success" ? "error" : toastKind);
        return false;
      }
      if (closeFeedback) {
        const modal = document.querySelector("[data-feedback-modal]");
        if (modal instanceof HTMLElement) { modal.classList.add("hidden"); }
        // Clear the feedback form so the next time the user opens the
        // modal they start from a blank slate, and so the act of saving
        // is visible (textarea no longer has their message). The
        // feedback form is identified by id="feedback-form" in
        // internal/templates/layout.templ.
        const feedbackForm = document.getElementById("feedback-form");
        if (feedbackForm instanceof HTMLFormElement) {
          feedbackForm.reset();
        }
        // Render the toast immediately rather than queueing it via
        // savePendingToast. The previous code queued it, but no page
        // nav fires after the close-feedback path so the queued toast
        // never displayed — the user submitted feedback and saw
        // nothing. Immediate showToast gives the confirmation the
        // user expects. Issue: feedback save closed the modal but
        // offered no confirmation.
        if (toastMessage) {
          showToast(toastMessage, toastKind);
        }
      }
      if (refreshCalendarMonth) {
        refreshCalendarGrid(refreshCalendarMonth);
      }
      if (button.closest("form") instanceof HTMLFormElement && response.ok) {
        clearDraftForForm(button.closest("form"));
      }
      if (toastMessage && !closeFeedback) {
        savePendingToast({ message: toastMessage, kind: toastKind });
      }
      // Inline render: if the form opts into data-results-target and the
      // response has no redirect, write the response body into the target
      // element and re-run the page-load initializers over that subtree.
      // Mirrors the browse-view refresh pattern at ~L3678. Opt-in only —
      // forms without the attribute keep the legacy toast-only path.
      // Issue #134: scan/quality buttons render into #settings-orphan-results
      // and #settings-quality-results via this convention.
      const resultsTargetSelector = (form.dataset && form.dataset.resultsTarget) || "";
      if (resultsTargetSelector && !redirectTo && responseOk) {
        const target = document.querySelector(resultsTargetSelector);
        if (target instanceof HTMLElement) {
          const html = await response.text();
          target.innerHTML = html;
          initializeDynamicContent(target);
        }
      }
      const requestState = {
        scrollX: window.scrollX,
        scrollY: window.scrollY,
      };
      rememberRedirectState(submitter || form, redirectTo || window.location.pathname, "", requestState);
      if (redirectTo) {
        window.location.assign(redirectTo);
      }
      return true;
    } finally {
      setBusyState(submitter || form, false);
      setBusyGroupState(submitter || form, false);
    }
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

  // Overlay modal helpers — restored from pre-issue-117 because the native
  // <dialog> element introduced in #117 caused focus-event reentry into
  // WebView2 when a native Save/Open dialog opened from inside the modal.
  // The native dialog hosts the focus event itself, so a separate
  // Chromium.Focus() call from the Wails onFocus hook crashes the
  // WebView2 control with the Chrome_WidgetWin_0 = 1412 class-cleanup
  // error. We use a plain <div role="dialog" aria-modal="true">
  // overlay, restore focus trap + ESC close manually, and keep
  // pointer/focus on background inert via Tailwind's z-index + backdrop.
  const OVERLAY_MODAL_FOCUSABLE = [
    'a[href]',
    'area[href]',
    'button:not([disabled])',
    'input:not([disabled]):not([type="hidden"])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    '[tabindex]:not([tabindex="-1"])',
  ].join(",");

  let overlayModalRestoreFocus = null;

  function showOverlayModal(modal) {
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    overlayModalRestoreFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    const focusable = modal.querySelectorAll(OVERLAY_MODAL_FOCUSABLE);
    if (focusable.length > 0) {
      focusable[0].focus();
    } else {
      modal.setAttribute("tabindex", "-1");
      modal.focus();
    }
    document.addEventListener("keydown", overlayModalKeydown, true);
  }

  function hideOverlayModal(modal) {
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    document.removeEventListener("keydown", overlayModalKeydown, true);
    if (overlayModalRestoreFocus instanceof HTMLElement) {
      overlayModalRestoreFocus.focus();
      overlayModalRestoreFocus = null;
    } else {
      overlayModalRestoreFocus = null;
    }
  }

  function overlayModalKeydown(event) {
    if (event.key !== "Tab") {
      return;
    }
    const openModal = document.querySelector('[role="dialog"][aria-modal="true"]:not(.hidden)');
    if (!(openModal instanceof HTMLElement)) {
      return;
    }
    const focusable = Array.from(openModal.querySelectorAll(OVERLAY_MODAL_FOCUSABLE)).filter(
      (el) => !el.hasAttribute("disabled") && el.offsetParent !== null,
    );
    if (focusable.length === 0) {
      event.preventDefault();
      return;
    }
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;
    if (event.shiftKey && active === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
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

  function googleCalendarPreferencesModal() {
    const modal = document.querySelector("[data-google-calendar-preferences-modal]");
    return modal instanceof HTMLElement ? modal : null;
  }

  function openPrintConfigModal() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    seedPrintRecordSelectionFromBrowse();
    syncPrintScopeState();
    applyPrintRecordFilter();
    applyPrintBuriedFilter();
    showOverlayModal(modal);
  }

  function closePrintConfigModal() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    hideOverlayModal(modal);
  }

  function openGoogleCalendarPreferencesModal() {
    const modal = googleCalendarPreferencesModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    syncGoogleCalendarPreview();
    showOverlayModal(modal);
  }

  function closeGoogleCalendarPreferencesModal() {
    const modal = googleCalendarPreferencesModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    hideOverlayModal(modal);
  }

  function syncGoogleCalendarPreview() {
    const titleTarget = document.querySelector("[data-google-pref-preview-title]");
    const timeTarget = document.querySelector("[data-google-pref-preview-time]");
    if (!(titleTarget instanceof HTMLElement) || !(timeTarget instanceof HTMLElement)) {
      return;
    }
    const titlePreset = document.querySelector('input[name="title_preset"]:checked')?.value || "memorial_full_name";
    const sampleName = "Capt. John Smith";
    const sampleDisplayID = "STC38-00001";
    if (titlePreset === "full_name_memorial") {
      titleTarget.textContent = `${sampleName} Memorial Anniversary`;
    } else if (titlePreset === "display_id_full_name") {
      titleTarget.textContent = `${sampleDisplayID} • ${sampleName}`;
    } else {
      titleTarget.textContent = `Memorial Anniversary: ${sampleName}`;
    }
    const startTimeInput = document.querySelector("[data-google-pref-start-time]");
    const startTime = startTimeInput instanceof HTMLInputElement && startTimeInput.value ? startTimeInput.value : "09:00";
    timeTarget.textContent = `Start: ${startTime} America/Chicago`;
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
    showOverlayModal(modal);
  }

  function closeFeedbackModal() {
    const modal = feedbackModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    hideOverlayModal(modal);
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
    const scope = (form.querySelector('input[name="scope"]:checked')?.value || "all").trim();
    const selectedIDs = scope !== "selected"
      ? []
      : Array.from(form.querySelectorAll('[data-print-record-checkbox]:checked'))
          .map((input) => Number.parseInt(input.value || "", 10))
          .filter((value) => Number.isInteger(value) && value > 0);
    const selectedFilterValues = (family) => Array.from(form.querySelectorAll(`[data-print-filter-checkbox][data-print-filter-family="${family}"]:checked`))
      .map((input) => (input instanceof HTMLInputElement ? input.value.trim() : ""))
      .filter((value) => value !== "");
    return {
      scope,
      sortBy: (form.querySelector('input[name="sort_by"]:checked')?.value || "last_name").trim(),
      groupByUnit: form.querySelector('input[name="group_by_unit"]')?.checked === true,
      groupByPensionState: form.querySelector('input[name="group_by_pension_state"]')?.checked === true,
      groupByConfederateHomeStatus: form.querySelector('input[name="group_by_confederate_home_status"]')?.checked === true,
      groupByBuriedIn: form.querySelector('input[name="group_by_buried_in"]')?.checked === true,
      filterBuriedIn: selectedFilterValues("buried-in"),
      filterEntryTypes: selectedFilterValues("entry-type"),
      filterUnits: selectedFilterValues("unit"),
      filterPensionStates: selectedFilterValues("pension-state"),
      filterConfederateHomeStatuses: selectedFilterValues("confederate-home-status"),
      exportAll: scope === "all",
      selectedIds: selectedIDs,
    };
  }

  function shareStatusTarget() {
    const target = document.getElementById("share-status");
    return target instanceof HTMLElement ? target : null;
  }

  function seedPrintRecordSelectionFromBrowse() {
    const form = printConfigForm();
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const selected = new Set(loadBrowseSelection());
    if (selected.size === 0) {
      return;
    }
    const checkboxes = Array.from(form.querySelectorAll("[data-print-record-checkbox]")).filter((checkbox) => checkbox instanceof HTMLInputElement);
    if (checkboxes.every((checkbox) => !checkbox.checked)) {
      checkboxes.forEach((checkbox) => {
        checkbox.checked = selected.has(Number.parseInt(checkbox.value || "", 10));
      });
      const selectedScope = form.querySelector('[data-print-scope-value][value="selected"]');
      if (selectedScope instanceof HTMLInputElement) {
        selectedScope.checked = true;
      }
    }
  }

  function openPrintConfigFromQuery() {
    const params = new URLSearchParams(window.location.search || "");
    if (params.get("openPrintConfig") !== "1") {
      return;
    }
    openPrintConfigModal();
    params.delete("openPrintConfig");
    const query = params.toString();
    const nextURL = `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash || ""}`;
    window.history.replaceState(window.history.state, "", nextURL);
  }

  function syncPrintScopeState() {
    const form = printConfigForm();
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scope = (form.querySelector('input[name="scope"]:checked')?.value || "all").trim();
    const picker = form.querySelector("[data-print-record-picker]");
    const recordFilter = form.querySelector("[data-print-record-filter]");
    const recordCheckboxes = form.querySelectorAll("[data-print-record-checkbox]");
    const filterPanel = form.querySelector("[data-print-filter-panel]");
    const structuredFilterInputs = form.querySelectorAll("[data-print-filter-checkbox], [data-print-buried-filter]");
    if (picker instanceof HTMLElement) {
      picker.classList.toggle("opacity-60", scope !== "selected");
    }
    if (recordFilter instanceof HTMLInputElement) {
      recordFilter.disabled = scope !== "selected";
    }
    recordCheckboxes.forEach((checkbox) => {
      if (checkbox instanceof HTMLInputElement) {
        checkbox.disabled = scope !== "selected";
      }
    });
    if (filterPanel instanceof HTMLElement) {
      filterPanel.classList.toggle("opacity-60", scope !== "filtered");
    }
    structuredFilterInputs.forEach((input) => {
      if (input instanceof HTMLInputElement) {
        input.disabled = scope !== "filtered";
      }
    });
  }

  function applyPrintRecordFilter() {
    const form = printConfigForm();
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scope = (form.querySelector('input[name="scope"]:checked')?.value || "all").trim();
    const query = (form.querySelector("[data-print-record-filter]")?.value || "").trim().toLowerCase();
    form.querySelectorAll("[data-print-record-option]").forEach((option) => {
      if (!(option instanceof HTMLElement)) {
        return;
      }
      const search = (option.getAttribute("data-print-record-search") || "").toLowerCase();
      const visible = scope !== "selected" || query === "" || search.includes(query);
      option.classList.toggle("hidden", !visible);
    });
  }

  function applyPrintBuriedFilter() {
    const form = printConfigForm();
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scope = (form.querySelector('input[name="scope"]:checked')?.value || "all").trim();
    const query = (form.querySelector("[data-print-buried-filter]")?.value || "").trim().toLowerCase();
    form.querySelectorAll("[data-print-buried-option]").forEach((option) => {
      if (!(option instanceof HTMLElement)) {
        return;
      }
      const search = (option.getAttribute("data-print-buried-search") || "").toLowerCase();
      const visible = scope !== "filtered" || query === "" || search.includes(query);
      option.classList.toggle("hidden", !visible);
    });
  }

  async function submitPrintConfig(form, trigger) {
    return; // deprecated — modal form is now plain htmx; kept stub for back-compat
  }

  document.addEventListener("DOMContentLoaded", () => {
    // Option C: strip pass deleted. htmx keeps running for the GET
    // polling fragments (/jobs/active, /jobs/{id}) — it doesn't
    // double-fire because no hx-post / hx-get on a click handler
    // exists. dispatchDixieDataForm intercepts form submits and
    // data-dixie-submit / data-merge-review-action clicks.
    ensureResponsiveLayoutWatcher();
    applyResponsiveLayout(document);
    initializeTabs();
    initializeDraftForms();
    initializeEntryTypeForms();
    initializeLiveCounts(document);
    initializeFloatingNav();
    initializeBrowseFilterDrawer();
    applyCalendarAnniversaryDensity();
    syncPrintScopeState();
    applyPrintRecordFilter();
    applyPrintBuriedFilter();
    syncGoogleCalendarPreview();
    restoreRedirectState();
    restorePendingToast();
    applySmartBackLabels();
    rememberRecentRecordFromPage();
    hydrateRecentSearchResults();
    initializeBrowseView();
    openPrintConfigFromQuery();
    // Re-init swapped subtrees after htmx polling swaps.
    if (typeof window !== "undefined" && window.htmx && typeof window.htmx.on === "function") {
      window.htmx.on("htmx:load", (evt) => {
        const target = evt.detail && evt.detail.elt;
        if (target instanceof HTMLElement) {
          initializeDynamicContent(target);
        }
      });
    }
    window.requestAnimationFrame(() => clampPopoutPanels(document));
  });

  document.addEventListener("click", (event) => {
    if (event.target && typeof event.target.closest === "function" && event.target.closest("summary")) {
      window.requestAnimationFrame(() => clampPopoutPanels(document));
    }
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
    const openGoogleCalendarPreferences = event.target.closest("[data-google-calendar-preferences-open]");
    if (openGoogleCalendarPreferences) {
      event.preventDefault();
      openGoogleCalendarPreferencesModal();
      return;
    }
    const clearBrowseSelection = event.target.closest("[data-browse-clear-selection]");
    if (clearBrowseSelection instanceof HTMLButtonElement) {
      event.preventDefault();
      saveBrowseSelection([]);
      applyBrowseSelection(document);
      return;
    }
    const resetBrowse = event.target.closest("[data-browse-reset]");
    if (resetBrowse instanceof HTMLButtonElement) {
      event.preventDefault();
      saveBrowseState(null);
      const resetPath = resetBrowse.getAttribute("data-browse-reset-path") || "/browse";
      window.location.assign(resetPath);
      return;
    }
    const anniversaryDensityToggle = event.target.closest("[data-calendar-anniversary-density-toggle]");
    if (anniversaryDensityToggle instanceof HTMLButtonElement) {
      event.preventDefault();
      saveCalendarAnniversaryDensity(anniversaryDensityToggle.getAttribute("data-calendar-anniversary-density-toggle") || "expanded");
      applyCalendarAnniversaryDensity(document);
      return;
    }
    const layoutModeToggle = event.target.closest("[data-layout-mode-option]");
    if (layoutModeToggle instanceof HTMLButtonElement) {
      event.preventDefault();
      saveLayoutModePreference(layoutModeToggle.getAttribute("data-layout-mode-option") || "auto");
      applyResponsiveLayout(document);
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
    const closeGoogleCalendarPreferences = event.target.closest("[data-google-calendar-preferences-close]");
    if (closeGoogleCalendarPreferences) {
      event.preventDefault();
      closeGoogleCalendarPreferencesModal();
      return;
    }
    if (event.target instanceof HTMLInputElement && event.target.matches("[data-print-scope-value]")) {
      syncPrintScopeState();
      applyPrintRecordFilter();
      applyPrintBuriedFilter();
      return;
    }
    if ((event.target instanceof HTMLInputElement || event.target instanceof HTMLSelectElement) && event.target.matches("[data-google-pref-input]")) {
      syncGoogleCalendarPreview();
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
    const browseRow = event.target.closest("[data-browse-row-href]");
    if (browseRow instanceof HTMLElement && !event.target.closest("a, button, input, label, select, textarea")) {
      const href = browseRow.getAttribute("data-browse-row-href");
      if (href) {
        event.preventDefault();
        pushBackSnapshot();
        window.location.assign(href);
        return;
      }
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
        const result = persistDraftForForm(form);
        setRecordPersistenceState(form, result.hasDraft ? "dirty" : "clean");
      }
      return;
    }
    const recordRemove = event.target.closest("[data-record-remove]");
    if (recordRemove) {
      event.preventDefault();
      removeRecordRow(recordRemove);
      const form = recordRemove.closest("form");
      if (form instanceof HTMLFormElement) {
        const result = persistDraftForForm(form);
        setRecordPersistenceState(form, result.hasDraft ? "dirty" : "clean");
      }
      return;
    }
    const clearDraftTrigger = event.target.closest("[data-clear-draft-trigger]");
    if (clearDraftTrigger instanceof HTMLElement) {
      event.preventDefault();
      showDraftDeleteConfirmation(clearDraftTrigger.closest("form"), clearDraftTrigger.getAttribute("data-clear-draft-trigger"));
      return;
    }
    const confirmClearDraft = event.target.closest("[data-confirm-clear-draft]");
    if (confirmClearDraft instanceof HTMLElement) {
      event.preventDefault();
      confirmDeleteDraftFromControl(confirmClearDraft);
      return;
    }
    const cancelClearDraft = event.target.closest("[data-cancel-clear-draft]");
    if (cancelClearDraft instanceof HTMLElement) {
      event.preventDefault();
      hideDraftDeleteConfirmation(cancelClearDraft.closest("form"), cancelClearDraft.getAttribute("data-cancel-clear-draft"));
      return;
    }
    const undoClearedDraft = event.target.closest("[data-undo-cleared-draft]");
    if (undoClearedDraft instanceof HTMLElement) {
      event.preventDefault();
      undoDeletedDraftFromControl(undoClearedDraft);
      return;
    }
    const reapplyStaleDraft = event.target.closest("[data-reapply-stale-draft]");
    if (reapplyStaleDraft instanceof HTMLElement) {
      event.preventDefault();
      reapplyStaleDraftFromControl(reapplyStaleDraft);
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
    // Option C dispatcher: intercepts form submits and clicks on
    // data-dixie-submit / data-merge-review-action. The legacy
    // hx-post / hx-delete / data-hx-* selectors remain here during
    // the templ retag (Commits 6–14); after the last templ file,
    // Option C: intercept clicks on data-dixie-submit + data-merge-review-action.
// hx-post / hx-delete / data-hx-* selectors dropped after the templ
// retag (every template uses data-dixie-submit now).
    const submitTrigger = event.target.closest("[data-dixie-submit], [data-merge-review-action]");
    if (submitTrigger instanceof HTMLElement && !(submitTrigger instanceof HTMLFormElement)) {
      event.preventDefault();
      dispatchDixieDataForm(submitTrigger);
      return;
    }
  });

  document.addEventListener("submit", (event) => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    if (!form.matches("[data-dixie-submit]")) {
      return;
    }
    event.preventDefault();
    dispatchDixieDataForm(event.submitter instanceof HTMLElement ? event.submitter : form);
  });

  // triggerInputRequest removed in Option C — no hx-trigger="keyup" /
  // "input" / "changed" use cases exist outside of polling fragments,
  // and polling is owned by htmx.
  document.addEventListener("focusin", (event) => {
    if (event.target === quickSearchInput()) {
      invalidateRecentSearchHydration();
    }
  });
  document.addEventListener("input", (event) => {
    if (event.target === quickSearchInput()) {
      invalidateRecentSearchHydration();
    }
  });
  document.addEventListener("focusout", (event) => {
    if (event.target !== quickSearchInput()) {
      return;
    }
    window.requestAnimationFrame(() => {
      const queryInput = quickSearchInput();
      if (!(queryInput instanceof HTMLInputElement) || document.activeElement === queryInput) {
        return;
      }
      hydrateRecentSearchResults();
    });
  });
  document.addEventListener("input", (event) => {
    const form = event.target.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      const result = persistDraftForForm(form);
      setRecordPersistenceState(form, result.hasDraft ? "dirty" : "clean");
    }
  });

  document.addEventListener("input", (event) => {
    if (event.target instanceof HTMLInputElement && event.target.matches("[data-print-record-filter]")) {
      applyPrintRecordFilter();
    }
  });
  document.addEventListener("input", (event) => {
    if (event.target instanceof HTMLInputElement && event.target.matches("[data-print-buried-filter]")) {
      applyPrintBuriedFilter();
    }
  });
  document.addEventListener("change", (event) => {
    const form = event.target.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      const result = persistDraftForForm(form);
      setRecordPersistenceState(form, result.hasDraft ? "dirty" : "clean");
    }
  });
  document.addEventListener("change", (event) => {
    const pdfInput = event.target.closest("[data-pdf-pref-key]");
    if (pdfInput instanceof HTMLElement) {
      const form = pdfInput.closest("form[data-pdf-pref-scope]");
      if (form instanceof HTMLFormElement) {
        persistPDFPreferences(form);
      }
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
  document.addEventListener("submit", (event) => {
    const form = event.target;
    if (form instanceof HTMLFormElement && form.matches("form[data-pdf-pref-scope]")) {
      persistPDFPreferences(form);
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
    const browseFilter = event.target.closest("[data-browse-filter-input]");
    if (browseFilter instanceof HTMLElement) {
      const form = browseFilter.closest("form");
      const pageField = form?.querySelector("[data-browse-page-input]");
      if (pageField instanceof HTMLInputElement) {
        pageField.value = "1";
      }
      if (form instanceof HTMLFormElement) {
        saveBrowseState(currentBrowseStateFromForm(form));
        // The browse filters form has hx-get / hx-target on the <form>
        // element but does not declare hx-trigger="change" on the
        // inputs. The change handler above resets paging + persists
        // state; we now also fire a fetch + swap into #browse-results
        // to keep the panel refreshed. Debounce 200ms so rapid
        // filter changes (e.g. typing in a select) don't fire a
        // fetch storm; matches the legacy queueRequest delay.
        const url = form.getAttribute("hx-get") || form.action;
        const targetSelector = form.getAttribute("hx-target") || "#browse-results";
        if (!url) { return; }
        clearTimeout(window.__dixieBrowseFilterTimer);
        window.__dixieBrowseFilterTimer = window.setTimeout(() => {
          (async () => {
            const params = new URLSearchParams(new FormData(form));
            try {
              const response = await fetch(`${url}?${params.toString()}`, {
                method: "GET",
                headers: { "X-Requested-With": "DixieData" },
              });
              const html = await response.text();
              const target = document.querySelector(targetSelector);
              if (target instanceof HTMLElement) {
                target.innerHTML = html;
                initializeDynamicContent(target);
              }
            } catch (error) {
              showToast("Browse refresh failed.", "error");
            }
          })();
        }, 200);
      }
      return;
    }
    const browseColumnToggle = event.target.closest("[data-browse-column-toggle]");
    if (browseColumnToggle instanceof HTMLInputElement) {
      const enabled = Array.from(document.querySelectorAll("[data-browse-column-toggle]"))
        .filter((input) => input instanceof HTMLInputElement && input.checked)
        .map((input) => input.value);
      saveBrowseColumns(enabled);
      applyBrowseColumns(document);
      return;
    }
    const browseSelect = event.target.closest("[data-browse-select]");
    if (browseSelect instanceof HTMLInputElement) {
      const id = Number.parseInt(browseSelect.value || "", 10);
      const selected = new Set(loadBrowseSelection());
      if (Number.isInteger(id) && id > 0) {
        if (browseSelect.checked) {
          selected.add(id);
        } else {
          selected.delete(id);
        }
      }
      saveBrowseSelection(Array.from(selected));
      updateBrowseSelectionStatus(document);
    }
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
      closeGoogleCalendarPreferencesModal();
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
    const googlePreferences = googleCalendarPreferencesModal();
    if (googlePreferences && event.target === googlePreferences) {
      closeGoogleCalendarPreferencesModal();
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
    clampPopoutPanels(document);
  });
  document.addEventListener(
    "toggle",
    (event) => {
      if (event.target && event.target.tagName === "DETAILS") {
        window.requestAnimationFrame(() => clampPopoutPanels(document));
      }
    },
    true,
  );
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
