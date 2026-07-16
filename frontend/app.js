(() => {
  // eventTargetElement narrows the loose `EventTarget` returned
  // by Document / Window event handlers to the actual Element the
  // click landed on (or null). The cost: one typeof check inline
  // at each handler's first `event.target` read. The benefit:
  // TypeScript stops flagging the 60+ `eventTargetElement(event)?.closest(...)`
  // sites in the DOMContentLoaded install block as TS2339, and the
  // runtime fails safe on synthetic events whose target is not
  // an Element (Text node / Window — DOM does not raise those
  // from a click, but the guard makes the slice-3 narrowing
  // explicit instead of relying on every site to add its own
  // `instanceof Element` check).
  /**
   * @param {Event | null | undefined} event
   * @returns {Element | null}
   */
  const eventTargetElement = (event) => {
    // Dual-mode guard. In the browser `Element` is a DOM global; the
    // Node-based go test JS harnesses (`browse_frontend_harness.js`)
    // don't define `Element` as a global — only `HTMLElement` +
    // typed subclasses. Fall back to a duck-type check on `EventTarget`
    // + `closest` so the harness can exercise the click delegation
    // paths against the mocked `HTMLElement.closest()`. In the
    // browser both checks pass and behavior is unchanged.
    if (
      event
      && event.target
      && typeof EventTarget !== "undefined"
      && typeof (/** @type {any} */ (event.target).closest) === "function"
    ) {
      return /** @type {any} */ (event.target);
    }
    return null;
  };
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
  /** @type {MediaQueryList | null} */
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
  /** @type {{ target: EventTarget | null, selectionText: string }} */
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

  /** @param {unknown[]} stack */
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

  /** @param {unknown} ids */
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

  /**
   * @param {string} key
   * @param {unknown} fallback
   */
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

  /**
   * @param {string} key
   * @param {unknown} value
   */
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

  /** @param {unknown} state */
  function saveBrowseState(state) {
    saveJSONStorage(browseStateStorageKey, state);
  }

  function loadBrowseColumns() {
    const value = loadJSONStorage(browseColumnsStorageKey, defaultBrowseColumns);
    return Array.isArray(value) && value.length > 0 ? value : defaultBrowseColumns.slice();
  }

  /** @param {unknown} columns */
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

  /** @param {string} mode */
  function saveCalendarAnniversaryDensity(mode) {
    saveJSONStorage(calendarAnniversaryDensityStorageKey, mode === "compact" ? "compact" : "expanded");
  }

  function loadLayoutModePreference() {
    const value = loadJSONStorage(layoutModeStorageKey, "auto");
    return value === "relaxed" || value === "split-screen" ? value : "auto";
  }

  /** @param {string} mode */
  function saveLayoutModePreference(mode) {
    const normalized = mode === "relaxed" || mode === "split-screen" ? mode : "auto";
    saveJSONStorage(layoutModeStorageKey, normalized);
    return normalized;
  }

  /** @param {string} preference */
  function resolveResponsiveLayoutMode(preference) {
    if (preference === "relaxed" || preference === "split-screen") {
      return preference;
    }
    if (typeof window.matchMedia === "function") {
      return window.matchMedia(`(max-width: ${splitScreenBreakpointPx}px)`).matches ? "split-screen" : "relaxed";
    }
    return window.innerWidth <= splitScreenBreakpointPx ? "split-screen" : "relaxed";
  }

  /** @param {string} mode */
  function layoutModeLabel(mode) {
    return mode === "split-screen" ? "Split-screen" : "Relaxed";
  }

  /** @param {string} preference */
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

  /**
   * @param {Document} root
   * @param {string} preference
   * @param {string} mode
   */
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

  /** @param {Document} [root] */
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
    measureFloatingDockHeight(doc, html);
    clampPopoutPanels(doc);
  }

  // measureFloatingDockHeight sets the --floating-dock-height CSS
  // variable on <html> AND directly updates .app-shell padding-bottom
  // from the dock's measured height. The CSS variable is exposed
  // for any future consumer (toast region offset, etc.). The direct
  // DOM write on .app-shell is the binding effect today because
  // the Tailwind minifier strips unused var() references from
  // .app-shell padding-bottom — see Common Bug #4.14.
  //
  // Per docs/COMMON_BUGS.md §4.14 the previous approach (hand-coded
  // padding-bottom + manual dock repositioning) regressed 5 times;
  // measuring the dock at runtime is the prescribed fix.
  /**
   * @param {Document} doc
   * @param {HTMLElement} html
   */
  function measureFloatingDockHeight(doc, html) {
    const dock = doc.querySelector(".floating-dock");
    if (!(dock instanceof HTMLElement)) {
      return;
    }
    const rect = dock.getBoundingClientRect();
    if (!rect.height) {
      return;
    }
    // dock height + 3rem breathing room (matches the historical
    // baseline padding-bottom values, so users see no visual change
    // unless the dock actually grew/shrank).
    const heightPx = Math.ceil(rect.height) + 48;
    html.style.setProperty("--floating-dock-height", `${heightPx}px`);
    const appShell = doc.querySelector(".app-shell");
    if (!(appShell instanceof HTMLElement)) {
      return;
    }
    // Issue #235: only override the inline padding-bottom when the
    // measured value is LARGER than the CSS-computed padding-bottom.
    // The CSS baseline (.app-shell padding-bottom: 7.5rem / 9rem at
    // narrower viewports) already accommodates the standard 3-button
    // dock; unconditionally writing the measured value shrinks the
    // content area on first hydration and causes the visible scrollbar
    // shift + cursor swap when the page is a fresh archive (no
    // cached layout-mode preference to mask the reflow). When the
    // dock is taller than the baseline (e.g. wrapped buttons on a
    // narrow viewport) the inline write is necessary so the dock
    // never overlaps content.
    const computedPadding = parseFloat(window.getComputedStyle(appShell).paddingBottom) || 0;
    if (heightPx <= computedPadding) {
      return;
    }
    appShell.style.paddingBottom = `${heightPx}px`;
  }

  /** @param {Document} [root] */
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

  // Issue #476: a single shared popover placement helper used by
  // every top-nav popout (foldout, megamenu, dock panel, calendar
  // day popout). The pre-#476 behavior was that the foldout +
  // megamenu open() handlers in installFoldouts / installMegaMenus
  // showed the panel but never called clampPopoutPanels, and the
  // clamp helper's selector only targeted [data-popout-panel] —
  // so on a narrow window a panel anchored to a trigger near the
  // right edge would extend leftward off-screen, clipping the
  // first menuitem(s) past the viewport's left edge. The fix:
  // (1) placePopoutPanel measures the trigger + the panel and
  // computes a horizontal shift so the panel stays inside the
  // viewport; (2) every open() handler invokes placePopoutPanel
  // after panel.classList.remove("hidden"); (3) the dock panel's
  // templ inline onclick handler also calls it. clampPopoutPanels
  // stays as a defense-in-depth post-paint sweep for the
  // [data-popout-panel] family that opens via <details>/<summary>.
  /**
   * Place a popout panel inside the viewport relative to its trigger.
   * The panel is positioned via translateX (and translateY if it would
   * overflow the bottom edge) so the trigger's anchor point is
   * preserved — we shift the panel itself, not the trigger. Safe to
   * call repeatedly: any prior transform is cleared before the new
   * measurement runs.
   *
   * @param {HTMLElement | null} trigger
   * @param {HTMLElement | null} panel
   */
  function placePopoutPanel(trigger, panel) {
    const viewportPadding = 12;
    if (!(trigger instanceof HTMLElement) || !(panel instanceof HTMLElement)) {
      return;
    }
    // Reset any prior transform so the new measurement is against
    // the panel's natural anchored position.
    panel.style.removeProperty("transform");
    const panelRect = panel.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    let shiftX = 0;
    let shiftY = 0;
    // Smart horizontal placement. The panel is anchored right of the
    // trigger (the .foldout-panel + .mega-menu-panel rules use
    // `right-0 top-full` so the panel extends leftward from the
    // trigger's right edge). If the trigger sits near the right
    // edge of a narrow window, the panel can overflow the left edge
    // of the viewport — we shift it right until its left edge is at
    // least viewportPadding from the viewport's left edge. We never
    // shift the panel so far right that it would overflow the right
    // edge; the panel keeps the trigger-anchored position when
    // there's enough horizontal room.
    if (panelRect.right > viewportWidth - viewportPadding) {
      shiftX -= panelRect.right - (viewportWidth - viewportPadding);
    }
    if (panelRect.left + shiftX < viewportPadding) {
      shiftX += viewportPadding - (panelRect.left + shiftX);
    }
    // Vertical placement: if the panel would overflow the bottom
    // edge of the viewport (e.g. on a short window where the trigger
    // sits near the bottom), flip it above the trigger.
    if (panelRect.bottom > viewportHeight - viewportPadding) {
      const overflow = panelRect.bottom - (viewportHeight - viewportPadding);
      // First try shifting up by the overflow amount (keeps the
      // panel anchored above the trigger's bottom).
      shiftY -= overflow;
      // If that would push the panel above the viewport's top edge,
      // clamp so the top stays at viewportPadding.
      if (panelRect.top + shiftY < viewportPadding) {
        shiftY += viewportPadding - (panelRect.top + shiftY);
      }
    }
    if (Math.abs(shiftX) > 0.5 || Math.abs(shiftY) > 0.5) {
      const tx = Math.round(shiftX);
      const ty = Math.round(shiftY);
      // translate3d keeps the transform compositing on its own
      // layer so the panel doesn't repaint its background every
      // frame during the open transition.
      panel.style.transform = `translate3d(${tx}px, ${ty}px, 0)`;
      return;
    }
    panel.style.removeProperty("transform");
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

  /** @param {unknown} ids */
  function saveBrowseSelection(ids) {
    const normalized = Array.from(new Set((Array.isArray(ids) ? ids : []).filter((value) => Number.isInteger(value) && value > 0)));
    saveJSONStorage(browseSelectionStorageKey, normalized);
  }

  /** @param {string | null} scope */
  function loadPDFPreferences(scope) {
    if (!scope) {
      return {};
    }
    const value = loadJSONStorage(`${pdfPreferencesStoragePrefix}${scope}`, {});
    return value && typeof value === "object" ? value : {};
  }

  /**
   * @param {string | null} scope
   * @param {unknown} values
   */
  function savePDFPreferences(scope, values) {
    if (!scope) {
      return;
    }
    saveJSONStorage(`${pdfPreferencesStoragePrefix}${scope}`, values);
  }

  /**
   * @param {HTMLInputElement | HTMLSelectElement} input
   * @returns {string | boolean}
   */
  function pdfPreferenceValue(input) {
    if (input instanceof HTMLInputElement && input.type === "checkbox") {
      return input.checked;
    }
    return input.value;
  }

  /** @param {Element | null} form */
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

  /** @param {Element | null} form */
  function persistPDFPreferences(form) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scope = form.getAttribute("data-pdf-pref-scope");
    /** @type {Record<string, string | number | boolean>} */
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

  // Issue #378 slice 3: research picker recents are persisted in
  // localStorage (key dixiedata.research.recents), capped at 10,
  // deduped by id, push-to-head. Mirrors the loadRecentRecords /
  // saveRecentRecords shape above so the storage policy is
  // consistent across both recent lists.
  const researchRecentsStorageKey = "dixiedata.research.recents";
  const researchRecentsStorageCap = 10;
  const researchRecentsHydrationState = { token: 0 };

  function loadResearchRecents() {
    try {
      const raw = window.localStorage.getItem(researchRecentsStorageKey);
      const parsed = raw ? JSON.parse(raw) : [];
      return Array.isArray(parsed) ? parsed.filter((value) => Number.isInteger(value) && value > 0) : [];
    } catch (error) {
      return [];
    }
  }

  /** @param {unknown} ids */
  function saveResearchRecents(ids) {
    try {
      const normalized = Array.from(new Set((Array.isArray(ids) ? ids : []).filter((value) => Number.isInteger(value) && value > 0))).slice(0, researchRecentsStorageCap);
      if (normalized.length === 0) {
        window.localStorage.removeItem(researchRecentsStorageKey);
        return;
      }
      window.localStorage.setItem(researchRecentsStorageKey, JSON.stringify(normalized));
    } catch (error) {
      // Ignore storage failures.
    }
  }

  function rememberResearchPickFromPage() {
    const detail = document.querySelector("[data-research-record-id]");
    if (!(detail instanceof HTMLElement)) {
      return;
    }
    const id = Number.parseInt(detail.getAttribute("data-research-record-id") || "", 10);
    if (!Number.isInteger(id) || id < 1) {
      return;
    }
    const next = [id].concat(loadResearchRecents().filter((value) => value !== id)).slice(0, researchRecentsStorageCap);
    saveResearchRecents(next);
  }

  // researchPickerNextKeyword returns the current NextAction the
  // picker is configured with so the recents-list forms redirect to
  // the right sub-page. The picker echoes it into a hidden form
  // field with name="next"; we read that off the first picker form
  // and fall back to "camaraderie" when no picker is on the page.
  //
  // Issue #487: the prior `document.querySelector("#" + "page.research.picker form input[name='next']")`
  // never matched. CSS parses `#page.research.picker` as a compound
  // selector requiring id="page" + class "research" + class "picker",
  // not the literal id "page.research.picker". Every picker
  // navigation silently fell back to "camaraderie" — the
  // Open Timeline / Open Research Log shortcuts landed on
  // /soldiers/{id}/camaraderie instead of the requested sub-page.
  // Use getElementById for the dotted-id lookup (matches the
  // sibling researchRecentsTarget() pattern at this same boundary)
  // then querySelector for the descendant form + input.
  function researchPickerNextKeyword() {
    const pageEl = document.getElementById("page.research.picker");
    if (!(pageEl instanceof HTMLElement)) {
      return "camaraderie";
    }
    const form = pageEl.querySelector("form input[name='next']");
    if (form instanceof HTMLInputElement && form.value) {
      return form.value;
    }
    return "camaraderie";
  }

  function researchRecentsTarget() {
    return document.getElementById("panel.research.picker.recent");
  }

  function researchRecentsEmptyState() {
    return document.querySelector("[data-research-recent-empty]");
  }

  async function hydrateResearchPickerRecents() {
    const emptyState = researchRecentsEmptyState();
    const target = researchRecentsTarget();
    if (!(emptyState instanceof HTMLElement) || !(target instanceof HTMLElement)) {
      return;
    }
    const ids = loadResearchRecents();
    if (ids.length === 0) {
      return;
    }
    const next = researchPickerNextKeyword();
    const token = researchRecentsHydrationState.token + 1;
    researchRecentsHydrationState.token = token;
    try {
      const response = await fetch("/research/recent?ids=" + encodeURIComponent(ids.join(",")) + "&next=" + encodeURIComponent(next), {
        headers: {
          "X-Requested-With": "fetch",
        },
      });
      if (!response.ok || researchRecentsHydrationState.token !== token) {
        return;
      }
      const html = await response.text();
      if (researchRecentsHydrationState.token !== token) {
        return;
      }
      const liveEmpty = researchRecentsEmptyState();
      const liveTarget = researchRecentsTarget();
      if (!(liveEmpty instanceof HTMLElement) || !(liveTarget instanceof HTMLElement)) {
        return;
      }
      // Replace the entire recents-region innerHTML (it is a
      // section wrapper holding either an empty <p> or a populated
      // <ul>). On a populated response, the empty <p> goes away
      // and the <ul> appears; on an empty response (no ids mapped
      // to a person), the server returns the empty-state again.
      liveTarget.innerHTML = html;
      initializeDynamicContent();
    } catch (error) {
      // Leave the empty state in place if the recent list cannot
      // be loaded.
    }
  }

  function quickSearchInput() {
    const input = document.querySelector('input[data-quick-search]');
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

  /** @param {HTMLElement} root */
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

  /** @param {string} path */
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
    if (/^\/soldiers\/\d+/.test(normalized)) {
      return "Back to Person Record";
    }
    if (normalized.startsWith("/soldiers/search") || normalized.startsWith("/soldiers?") || normalized === "/soldiers") {
      return "Back to Results";
    }
    if (normalized.startsWith("/browse")) {
      return "Back to Browse";
    }
    if (normalized.startsWith("/jobs")) {
      return "Back to Jobs";
    }
    if (normalized.startsWith("/settings")) {
      return "Back to Settings";
    }
    if (normalized.startsWith("/recovery")) {
      return "Back to Recovery";
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

  // Source record reorder highlight (UX pass). The form-submit
  // dispatcher stashes the moved record's ID in sessionStorage
  // just before navigating to the reload URL. On the next page
  // load this helper reads that flag, applies
  // [data-just-moved=\"true\"] to the matching <li>, and clears
  // the flag once the highlight animation completes. Single
  // flash per successful reorder; no flash if the user landed
  // here without a preceding reorder (e.g. a manual deep link).
  // The CSS animation lives in frontend/tailwind.css under
  // [data-source-record-id][data-just-moved=\"true\"].
  function flashLastMovedSourceRecord() {
    /** @type {string | null} */
    let movedId = null;
    try {
      movedId = window.sessionStorage.getItem("dixiedata.lastMovedSource");
    } catch (_) {
      return;
    }
    if (!movedId) {
      return;
    }
    const row = document.querySelector(
      `[data-source-record-id=\"${movedId}\"]`
    );
    if (!(row instanceof HTMLElement)) {
      // No matching row on this page — clear the flag and
      // bail. Could happen if the user navigates back to a
      // different soldier before the reload finishes.
      try {
        window.sessionStorage.removeItem("dixiedata.lastMovedSource");
      } catch (_) {}
      return;
    }
    // Defer to next frame so the browser has applied the page
    // load styles before the keyframe starts (otherwise the
    // first frame of the animation can flash without the
    // starting amber background).
    requestAnimationFrame(() => {
      row.setAttribute("data-just-moved", "true");
      row.scrollIntoView({ block: "nearest", behavior: "smooth" });
      const cleanup = () => {
        row.removeAttribute("data-just-moved");
        try {
          window.sessionStorage.removeItem("dixiedata.lastMovedSource");
        } catch (_) {}
      };
      // Primary cleanup: when the CSS transition ends.
      // Fallback: 2s timeout in case the transitionend event
      // never fires (e.g. the user navigates away mid-flash).
      /** @param {AnimationEvent | TransitionEvent} ev */
      const onEnd = (ev) => {
        if (ev && ev.target !== row) {
          return;
        }
        row.removeEventListener("transitionend", onEnd);
        row.removeEventListener("animationend", onEnd);
        cleanup();
      };
      row.addEventListener("transitionend", onEnd, { once: true });
      row.addEventListener("animationend", onEnd, { once: true });
      setTimeout(cleanup, 2000);
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

  /** @param {string} href */
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

  /** @param {Element} el */
  function closestParentForm(el) {
    if (el instanceof HTMLFormElement) {
      return null;
    }
    return el.closest("form");
  }

  /** @param {Element} el */
  function ownerForm(el) {
    if (el instanceof HTMLFormElement) {
      return el;
    }
    const form = closestParentForm(el);
    return form instanceof HTMLFormElement ? form : null;
  }

  /** @param {string} group */
  function selectedCompareEntries(group) {
    /** @type {{ id: string; label: string }[]} */
    const out = [];
    for (const checkbox of document.querySelectorAll(`[data-checkbox-group="${group}"][data-compare-select]:checked`)) {
      if (!(checkbox instanceof HTMLInputElement)) {
        continue;
      }
      if (!checkbox.value) {
        continue;
      }
      out.push({
        id: checkbox.value,
        label: checkbox.getAttribute("data-compare-label") || checkbox.value,
      });
    }
    return out;
  }

  /** @param {string} group */
  function selectedCompareIDs(group) {
    return selectedCompareEntries(group).map((entry) => entry.id);
  }

  /** @param {string} group */
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

  /** @param {Element} trigger */
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

  /**
   * @param {number} value
   * @param {number} min
   * @param {number} max
   */
  function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
  }

  /** @param {EventTarget | null} target */
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

  /** @param {EventTarget | null} target */
  function isEditableTextTarget(target) {
    return isTextInputTarget(target) || target instanceof HTMLElement && target.isContentEditable;
  }

  /** @param {EventTarget} target */
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
    const targetReadable = target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement ? target : null;
    const canCut = editable && selectionLen > 0 && !(targetReadable && targetReadable.readOnly) && !(targetReadable && targetReadable.disabled);
    const canCopy = selectionLen > 0 || !!textContextMenuState.selectionText;
    const canPaste = editable && !(targetReadable && targetReadable.readOnly) && !(targetReadable && targetReadable.disabled);
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

  /**
   * @param {EventTarget | null} target
   * @param {number} clientX
   * @param {number} clientY
   */
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

  /**
   * @param {EventTarget} target
   * @param {string} replacement
   */
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

  /** @param {string | null} action */
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

  /** @param {string | null} targetId */
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

  /** @param {string} message */
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

  /**
   * @param {number} nextZoom
   * @param {number} [pointerX]
   * @param {number} [pointerY]
   */
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

  /** @param {string} url */
  function cacheBustedImageURL(url) {
    if (!url) {
      return url;
    }
    const separator = url.includes("?") ? "&" : "?";
    return `${url}${separator}v=${Date.now()}`;
  }

  /**
   * @param {string} imageId
   * @param {string} baseUrl
   */
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
    document.querySelectorAll(`[data-image-preview-id="${imageId}"]`).forEach((button) => {
      if (button instanceof HTMLElement) {
        button.setAttribute("data-image-preview", refreshedUrl);
      }
    });
    imageViewerState.imageUrl = baseUrl;
    return refreshedUrl;
  }

  /** @param {string} direction */
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

  /**
   * @param {string | null} url
   * @param {string | null} caption
   * @param {string | null} fileName
   * @param {string | null} imageId
   */
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
    image.setAttribute("src", /** @type {string} */ (url));
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
  /**
   * @param {string | null} caption
   * @param {string | null} fileName
   */
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
  /** @param {string} value */
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

  /** @param {HTMLFormElement | null} form */
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

  /** @param {Element | null} el */
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

  /** @param {string} displayId */
  function normalizeScratchpadDisplayId(displayId) {
    const value = (displayId || "").trim();
    return value || "unfiled";
  }

  /** @param {string} displayId */
  function scratchpadContentKey(displayId) {
    return `dixiedata:scratchpad:${normalizeScratchpadDisplayId(displayId)}`;
  }

  /** @param {string} displayId */
  function loadLegacyScratchpadText(displayId) {
    try {
      return window.localStorage.getItem(scratchpadContentKey(displayId)) || "";
    } catch (error) {
      return "";
    }
  }

  /** @param {Element} trigger */
  async function openScratchpad(trigger) {
    const form = scratchpadFormFromElement(trigger);
    const displayId = scratchpadDisplayId(form) || pageScratchpadDisplayId();
    if (!displayId) {
      // Issue #535: the no-record-open branch now surfaces as
      // a toast (the prior setScratchpadStatus helper wrote to
      // a near-invisible bottom-dock <p data-floating-scratchpad-status>
      // that was hard to spot). Kind: "warning" matches the
      // existing toast vocabulary for "user can't proceed yet".
      showToast("Open a record with a saved Record ID before launching the scratch pad.", "warning");
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
      // Issue #535: success + failure both toast. The prior
      // helper wrote to the bottom-dock pill which was easy
      // to miss; a toast in the same corner as every other
      // action response is the consistent signal the user
      // asked for.
      if (response.ok) {
        showToast(message || "Scratch pad opened.", "success");
      } else {
        showToast(message || "Scratch pad failed to open.", "error");
      }
    } catch (error) {
      showToast("Scratch pad failed to open.", "error");
    } finally {
      setBusyState(trigger, false);
    }
  }

  /**
   * @param {string} group
   * @param {boolean} checked
   */
  function toggleCheckboxGroup(group, checked) {
    document.querySelectorAll(`[data-checkbox-group="${group}"]`).forEach((checkbox) => {
      if (checkbox instanceof HTMLInputElement) {
        checkbox.checked = checked;
      }
    });
  }

  /** @param {Element} button */
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

  /** @param {Element} button */
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

  /** @param {HTMLFormElement | null} form */
  function draftKeyForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    return form.getAttribute("data-draft-key") || "";
  }

  /** @param {HTMLFormElement | null} form */
  function draftStorageKeyForForm(form) {
    const key = draftKeyForForm(form);
    return key ? `dixiedata:${key}` : "";
  }

  /** @param {HTMLFormElement | null} form */
  function draftKindForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "new";
    }
    return form.getAttribute("data-record-persistence-kind") || "new";
  }

  /** @param {HTMLFormElement | null} form */
  function draftRecordVersionForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    return form.getAttribute("data-draft-record-version") || "";
  }

  /** @param {HTMLFormElement | null} form */
  function draftResetPathForForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return "";
    }
    return form.getAttribute("data-draft-reset-path") || "";
  }

  /** @param {Element} field */
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

  /** @param {HTMLFormElement | null} form */
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

  /** @param {{ draftKey?: string, payload?: string } | null} state */
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

  /** @param {HTMLFormElement | null} form */
  function deletedDraftStateForForm(form) {
    const state = loadDeletedDraftState();
    if (!state || state.draftKey !== draftKeyForForm(form)) {
      return null;
    }
    return state;
  }

  /** @param {HTMLFormElement | null} form */
  function clearDeletedDraftStateForForm(form) {
    if (deletedDraftStateForForm(form)) {
      clearDeletedDraftState();
    }
  }

  /** @param {HTMLFormElement | null} form */
  function formRecordRowCount(form) {
    if (!(form instanceof HTMLFormElement)) {
      return 1;
    }
    const count = form.querySelectorAll("[data-record-row]").length;
    return count > 0 ? count : 1;
  }

  /**
   * @param {HTMLFormElement | null} form
   * @param {number} targetCount
   */
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

  /** @param {Element & { value?: unknown }} field */
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

  /** @param {HTMLFormElement} form */
  /** @param {HTMLFormElement} form @returns {Record<string, string[]>} */
function serializeDraftFields(form) {
    /** @type {Record<string, string[]>} */
    const payload = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!isDraftableField(field)) {
        return;
      }
      if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement || field instanceof HTMLSelectElement)) {
        return;
      }
      if (!Object.prototype.hasOwnProperty.call(payload, field.name)) {
        payload[field.name] = [];
      }
      payload[field.name].push(draftFieldValue(field));
    });
    return payload;
  }

  /** @param {Record<string, unknown> | null | undefined} snapshot */
  function cloneDraftSnapshot(snapshot) {
    /** @type {Record<string, string[]>} */
    const clone = {};
    Object.entries(snapshot || {}).forEach(([name, values]) => {
      clone[name] = Array.isArray(values) ? values.map((value) => String(value ?? "")) : [];
    });
    return clone;
  }

  /**
   * @param {Record<string, unknown> | null | undefined} left
   * @param {Record<string, unknown> | null | undefined} right
   */
  function snapshotsEqual(left, right) {
    const names = new Set([...Object.keys(left || {}), ...Object.keys(right || {})]);
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

  /**
   * @param {Record<string, unknown> | null | undefined} base
   * @param {Record<string, unknown> | null | undefined} overrides
   */
  function mergeDraftSnapshot(base, overrides) {
    const merged = cloneDraftSnapshot(base || {});
    Object.entries(overrides || {}).forEach(([name, values]) => {
      merged[name] = Array.isArray(values) ? values.map((value) => String(value ?? "")) : [];
    });
    return merged;
  }

  /** @param {HTMLFormElement} form */
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

  /** @param {unknown} raw */
  function normalizeDraftSnapshot(raw) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
      return {};
    }
    /** @type {Record<string, string[]>} */
    const normalized = {};
    Object.entries(raw).forEach(([name, values]) => {
      if (!Array.isArray(values)) {
        return;
      }
      normalized[name] = values.map((value) => String(value ?? ""));
    });
    return normalized;
  }

  /** @param {Record<string, unknown> | null | undefined} snapshot */
  function calculateDraftRowCount(snapshot) {
    return Math.max(
      1,
      Array.isArray(snapshot?.record_type) ? snapshot.record_type.length : 0,
      Array.isArray(snapshot?.record_app_id) ? snapshot.record_app_id.length : 0,
      Array.isArray(snapshot?.record_details) ? snapshot.record_details.length : 0,
    );
  }

  /** @param {HTMLFormElement} form */
  function buildDraftPayload(form) {
    const kind = draftKindForForm(form);
    const currentFields = serializeDraftFields(form);
    if (kind === "edit") {
      const baseline = baselineStateForForm(form);
      /** @type {Record<string, string[]>} */
      const delta = {};
      let changed = false;
      const names = new Set([...Object.keys(baseline.fields || {}), ...Object.keys(currentFields || {})]);
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

  /** @param {HTMLFormElement} form */
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

  /**
   * @param {Element | null} field
   * @param {unknown} rawValue
   */
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

  /**
   * @param {string} name
   * @param {number} occurrence
   */
  function draftFieldLabel(name, occurrence) {
    /** @type {Record<string, string>} */
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

  /**
   * @param {HTMLFormElement} form
   * @param {Record<string, string[]> | null | undefined} baselineFields
   * @param {Record<string, string[]> | null | undefined} draftSnapshot
   */
  function buildDraftDiffEntries(form, baselineFields, draftSnapshot) {
    /** @type {Array<{ label: string, currentValue: string, localValue: string }>} */
    const entries = [];
    const names = Array.from(new Set([...Object.keys(baselineFields || {}), ...Object.keys(draftSnapshot || {})])).sort();
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

  /**
   * @param {HTMLFormElement} form
   * @param {Array<{ label: string, currentValue: string, localValue: string }>} entries
   * @param {boolean} showReapply
   */
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

  /**
   * @param {HTMLFormElement | null} form
   * @param {string} scope
   * @param {{ restoreTrigger?: boolean }} [options]
   */
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

  /**
   * @param {HTMLFormElement | null} form
   * @param {string} scope
   */
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

  /** @param {HTMLFormElement | null} form */
  function resetDraftDeleteConfirmations(form) {
    hideDraftDeleteConfirmation(form, "base", { restoreTrigger: false });
    hideDraftDeleteConfirmation(form, "stale", { restoreTrigger: false });
  }

  /**
   * @param {HTMLFormElement | null} form
   * @param {string} state
   */
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

  /**
   * @param {HTMLFormElement | null} form
   * @param {boolean} visible
   */
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

  /**
   * @param {HTMLFormElement} form
   * @param {string} state
   * @param {{ entries?: Array<{ label: string, currentValue: string, localValue: string }>, showUndo?: boolean }} [options]
   */
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

  /**
   * @param {HTMLFormElement} form
   * @param {{ rememberDeleted?: boolean, preserveDeletedState?: boolean }} [options]
   */
  function clearDraftForForm(form, options = {}) {
    const storageKey = draftStorageKeyForForm(form);
    if (!storageKey) {
      return;
    }
    /** @type {string | null} */
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

  /** @param {Element} control */
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

  /**
   * @param {HTMLFormElement} form
   * @param {Record<string, string[]> | null | undefined} snapshot
   * @param {number} rowCount
   */
  function applyDraftSnapshot(form, snapshot, rowCount) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    setRecordRowCount(form, rowCount || calculateDraftRowCount(snapshot));
    /** @type {Record<string, number>} */
    const cursors = {};
    form.querySelectorAll("input[name], textarea[name], select[name]").forEach((field) => {
      if (!isDraftableField(field)) {
        return;
      }
      // `.name` and `.value` live on HTMLInputElement / HTMLTextAreaElement
      // / HTMLSelectElement, not on Element. Narrowing here protects both
      // the runtime (misselectors throw undefined.foo) and the type checker
      // — TypeScript flagged the read-before-narrow as TS2339 in slice 2.
      if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement || field instanceof HTMLSelectElement)) {
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

  /** @param {HTMLFormElement} form */
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

  /** @param {HTMLFormElement} form */
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

  /** @param {Element} control */
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

  /** @param {Element} control */
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

  // Issue #321 slice 3.6: markdown editor live preview.
  // Hooks each [data-article-editor-preview] div to its
  // paired [data-article-editor-source-id="..."] textarea;
  // on input (250ms debounce), POST the body to
  // /articles/preview and replace the preview's innerHTML
  // with the sanitized HTML response. Empty body renders
  // the guidance message so the preview pane is never
  // blank.
  function initializeArticlePreview() {
    const modal = document.querySelector("[data-article-preview-modal]");
    if (!(modal instanceof HTMLElement)) return;
    const body = modal.querySelector("[data-article-preview-body]");
    if (!(body instanceof HTMLElement)) return;
    const source = document.getElementById("article-body");
    if (!(source instanceof HTMLTextAreaElement)) return;
    // Issue #573: shared debounce helper replaces the inline
    // clearTimeout/setTimeout pair on the Preview button (50ms
    // render-pulse). One debounce instance per modal open so a
    // second click mid-flight collapses into the trailing fire.
    // Typed as the helper's return shape (`(...args) => void` &
    // `{ cancel, schedule }`) so the `busyDebounce.cancel()` /
    // `busyDebounce()` calls type-check under strictNullChecks.
    const debounce = window.__dixieDebounce;
    if (!debounce) return;
    /** @type {((...args: any[]) => void) & { cancel: () => void; schedule: () => void } | null} */
    let busyDebounce = null;

    // The closures below (requestRender + the input handler) lose
    // the `instanceof` narrowing from the top of the function once
    // they close over `body` / `source` — TypeScript's control flow
    // narrowing doesn't carry into nested function bodies (see the
    // `updateTextContextMenuState` refactor in the strictNullChecks
    // slice 5b changelog for the canonical example). Re-narrow
    // inside the closures so the subsequent `.innerHTML` / `.value`
    // reads type-check. Without this, the strictNullChecks tsc step
    // in CI fails with TS18047 / TS2339.
    /** @type {HTMLElement} */
    const previewBody = body;
    /** @type {HTMLTextAreaElement} */
    const previewSource = source;
    async function requestRender() {
      const value = previewSource.value || "";
      const params = new URLSearchParams();
      params.append("body", value);
      /** @type {RequestInit} */
      const opts = {
        method: "POST",
        body: params.toString(),
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
      };
      // Wails v2.12.0 asset server (wails.localhost) strips
      // multipart/form-data bodies on POST; the dispatcher
      // uses URLSearchParams for the same reason (app.js
      // dispatchDixieDataForm FormData branch). We don't
      // need the method-override header here because
      // /articles/preview is POST-only; we just need the
      // body to survive.
      try {
        const resp = await fetch("/articles/preview", opts);
        if (!resp.ok) {
          previewBody.innerHTML = "<p class=\"text-sm text-rose-600\">Preview request failed (" + resp.status + ").</p>";
          return;
        }
        const html = await resp.text();
        previewBody.innerHTML = html || "<p class=\"text-sm text-slate-500\">Type Markdown in the editor and click Preview to see it rendered here.</p>";
      } catch (err) {
        previewBody.innerHTML = "<p class=\"text-sm text-rose-600\">Preview request failed.</p>";
      }
    }

    document.querySelectorAll("[data-article-preview-open]").forEach((trigger) => {
      if (!(trigger instanceof HTMLElement)) return;
      trigger.addEventListener("click", (event) => {
        event.preventDefault();
        showOverlayModal(modal);
        if (busyDebounce && typeof busyDebounce.cancel === "function") {
          busyDebounce.cancel();
        }
        previewBody.innerHTML = "<p class=\"text-sm text-slate-500\">Rendering preview\u2026</p>";
        if (!busyDebounce) {
          busyDebounce = debounce(() => {
            requestRender();
          }, 50);
        }
        busyDebounce();
      });
    });

    document.querySelectorAll("[data-article-preview-close]").forEach((closer) => {
      if (!(closer instanceof HTMLElement)) return;
      closer.addEventListener("click", (event) => {
        event.preventDefault();
        hideOverlayModal(modal);
      });
    });

    // Escape-to-close: overlayModalKeydown handles Tab trapping;
    // Escape is handled here because the modal is editor-driven
    // (not form-driven) so the dispatcher does not see it.
    modal.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        hideOverlayModal(modal);
      }
    });
  }

  // initializeInventoryMetricsChart (issue #583 slice 3) paints
  // the hand-rolled SVG line graph into the chart host the
  // templ partial renders, and wires the per-kind legend chips
  // so the user can toggle series visibility without an htmx
  // round-trip. The chart reads its data from the
  // data-inventory-metrics-by-kind attribute (JSON-encoded per
  // kind -> day -> count) and the data-active-kinds attribute
  // (comma-separated visible kinds).
  //
  // Series paths are grouped under data-inventory-metrics-series
  // (one <path> per kind) so the legend toggle can flip
  // display:none on the matching path. The whole render is
  // idempotent: the global guard __inventoryChartPainted plus
  // the empty-host check prevents double-painting on htmx swaps.
  /** Render the Activity metrics SVG line graph (issue #583). */
  function initializeInventoryMetricsChart() {
    const hosts = document.querySelectorAll("[data-inventory-metrics-svg-host]");
    hosts.forEach((host) => {
      if (!(host instanceof HTMLElement)) return;
      const wrapper = host.closest("[data-inventory-metrics-chart]");
      if (!(wrapper instanceof HTMLElement)) return;
      if (wrapper.__inventoryChartPainted === true) return;
      const raw = wrapper.getAttribute("data-inventory-metrics-by-kind");
      if (!raw) return;
      let byKind;
      try {
        byKind = JSON.parse(raw);
      } catch (err) {
        if (typeof console !== "undefined") {
          console.warn("inventory metrics: invalid JSON in data-inventory-metrics-by-kind", err);
        }
        return;
      }
      const kinds = readActiveKinds(wrapper);
      paintInventoryChart(host, wrapper, byKind, kinds);
      wireInventoryLegend(wrapper, kinds);
      ensureInventoryChartTooltip(wrapper);
      wireInventoryChartPoints(wrapper);
      wrapper.__inventoryChartPainted = true;
    });
  }

  // readActiveKinds reads the comma-separated kind list from
  // data-active-kinds. Falls back to the locked 5-kind default
  // if the attribute is absent or malformed so a templ bug
  // never produces an invisible chart.
  /**
   * @param {HTMLElement} wrapper chart wrapper
   * @returns {string[]} active kinds
   */
  function readActiveKinds(wrapper) {
    const raw = wrapper.getAttribute("data-active-kinds");
    if (!raw) {
      return ["soldier", "spouse", "linked", "event", "article"];
    }
    const out = raw
      .split(",")
      .map((k) => k.trim())
      .filter((k) => k.length > 0);
    if (out.length === 0) {
      return ["soldier", "spouse", "linked", "event", "article"];
    }
    return out;
  }

  // paintInventoryChart writes the SVG into the host. The host's
  // "Loading chart..." placeholder is removed; the SVG replaces
  // it. The function is the only writer of the host's children
  // so the templ placeholder cannot leak through.
  /**
   * @param {HTMLElement} host SVG mount host
   * @param {HTMLElement} wrapper chart wrapper
   * @param {Record<string, Record<string, number>>} byKind per-kind per-day counts
   * @param {string[]} activeKinds visible kinds
   */
  function paintInventoryChart(host, wrapper, byKind, activeKinds) {
    // Aggregate the union of all days across all kinds so the
    // x-axis spans the full activity window. Use a Set for O(1)
    // dedup; sort lexicographically (YYYY-MM-DD = chronological).
    const daySet = new Set();
    for (const kind of Object.keys(byKind)) {
      const inner = byKind[kind];
      if (!inner || typeof inner !== "object") continue;
      for (const day of Object.keys(inner)) {
        daySet.add(day);
      }
    }
    const days = Array.from(daySet).sort();
    const maxY = computeMaxY(byKind);
    // Issue #595 slice 2: the width source-of-truth is the host's
    // clientWidth at paint time, with a 480px fallback for the
    // first-paint window before the parent flex layout settles
    // (the Wails WebView2 sometimes reports 0 until the layout
    // reflow lands). The SVG additionally carries inline
    // `max-width: 100%; height: auto` so the browser clamps the
    // rendered SVG to the host's visible bounds even when the
    // JS-computed width is momentarily larger (e.g. on window
    // resize between paint and ResizeObserver callback). The
    // audit probe (audit/smoke_inventory_metrics.mjs) asserts
    // the SVG width attribute stays within the host bounds.
    const width = Math.max(host.clientWidth || 480, 320);
    const height = 192;
    const padding = { top: 16, right: 16, bottom: 28, left: 36 };
    const innerW = width - padding.left - padding.right;
    const innerH = height - padding.top - padding.bottom;
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
    svg.setAttribute("width", String(width));
    svg.setAttribute("height", String(height));
    // Browser-side clamp: never render the SVG wider than the
    // host, regardless of what `width` says. Combined with
    // viewBox + height: auto, the path coordinates inside
    // the viewBox (which respect `padding.left + innerW`) stay
    // inside the host's visible area on every viewport size.
    svg.setAttribute("style", "max-width: 100%; height: auto; display: block;");
    svg.setAttribute("preserveAspectRatio", "xMidYMid meet");
    svg.setAttribute("role", "img");
    svg.setAttribute("aria-label", "Activity metrics: per-kind entry counts over time");
    svg.setAttribute("data-inventory-metrics-svg", "");
    // Y-axis gridlines + labels.
    const yTicks = chooseYTicks(maxY);
    yTicks.forEach((tick) => {
      const y = padding.top + innerH * (1 - tick / maxY);
      const line = document.createElementNS(ns, "line");
      line.setAttribute("x1", String(padding.left));
      line.setAttribute("x2", String(padding.left + innerW));
      line.setAttribute("y1", String(y));
      line.setAttribute("y2", String(y));
      line.setAttribute("stroke", "rgba(120, 90, 60, 0.18)");
      line.setAttribute("stroke-width", "1");
      svg.appendChild(line);
      const label = document.createElementNS(ns, "text");
      label.setAttribute("x", String(padding.left - 6));
      label.setAttribute("y", String(y + 3));
      label.setAttribute("text-anchor", "end");
      label.setAttribute("font-size", "10");
      label.setAttribute("fill", "rgba(80, 60, 40, 0.7)");
      label.textContent = String(tick);
      svg.appendChild(label);
    });
    // X-axis labels: first, last, and one midpoint if there are
    // enough days to space them apart. Avoids a label collision
    // on a single-day chart.
    if (days.length === 1) {
      const x = padding.left + innerW / 2;
      const label = document.createElementNS(ns, "text");
      label.setAttribute("x", String(x));
      label.setAttribute("y", String(height - 8));
      label.setAttribute("text-anchor", "middle");
      label.setAttribute("font-size", "10");
      label.setAttribute("fill", "rgba(80, 60, 40, 0.7)");
      label.textContent = days[0];
      svg.appendChild(label);
    } else {
      [days[0], days[Math.floor(days.length / 2)], days[days.length - 1]].forEach((d) => {
        const x = padding.left + innerW * (dayIndex(d, days) / Math.max(days.length - 1, 1));
        const label = document.createElementNS(ns, "text");
        label.setAttribute("x", String(x));
        label.setAttribute("y", String(height - 8));
        label.setAttribute("text-anchor", "middle");
        label.setAttribute("font-size", "10");
        label.setAttribute("fill", "rgba(80, 60, 40, 0.7)");
        label.textContent = d;
        svg.appendChild(label);
      });
    }
    // Series paths, one per kind. Visibility is driven by the
    // active-kinds list -- a hidden series is display:none on
    // the <path>, not omitted from the DOM, so the legend toggle
    // can restore it without repainting.
    const seriesGroup = document.createElementNS(ns, "g");
    seriesGroup.setAttribute("data-inventory-metrics-series", "");
    // Issue #595 slice 3: hover hit-target layer. One <circle>
    // per (kind, day) data point, painted on top of the series
    // paths. The circle carries a JSON-encoded `data-point`
    // attribute (date + count + kind) so the hover handler
    // can read the payload without a closure lookup. The
    // hit-target circle is slightly larger than the visible
    // fill so the cursor doesn't need pixel-perfect aim.
    const pointsGroup = document.createElementNS(ns, "g");
    pointsGroup.setAttribute("data-inventory-metrics-points", "");
    const orderedKinds = ["soldier", "spouse", "linked", "event", "article"];
    orderedKinds.forEach((kind) => {
      const inner = byKind[kind];
      if (!inner || typeof inner !== "object") return;
      const d = linePath(kind, inner, days, padding, innerW, innerH, maxY);
      const path = document.createElementNS(ns, "path");
      path.setAttribute("d", d);
      path.setAttribute("fill", "none");
      path.setAttribute("stroke", kindColor(kind));
      path.setAttribute("stroke-width", "1.75");
      path.setAttribute("stroke-linecap", "round");
      path.setAttribute("stroke-linejoin", "round");
      path.setAttribute("data-inventory-metrics-series-kind", kind);
      if (!activeKinds.includes(kind)) {
        path.setAttribute("display", "none");
      }
      seriesGroup.appendChild(path);
      // Points for this kind. Skip days where the count is 0 so
      // hover doesn't fire on the flat baseline of inactive days.
      const denom = Math.max(days.length - 1, 1);
      for (let i = 0; i < days.length; i++) {
        const day = days[i];
        const count = Number(inner[day]) || 0;
        if (count === 0) continue;
        const x = padding.left + innerW * (i / denom);
        const y = padding.top + innerH * (1 - count / maxY);
        const circle = document.createElementNS(ns, "circle");
        circle.setAttribute("cx", String(x.toFixed(1)));
        circle.setAttribute("cy", String(y.toFixed(1)));
        circle.setAttribute("r", "6");
        circle.setAttribute("fill", kindColor(kind));
        circle.setAttribute("fill-opacity", "0.0");
        circle.setAttribute("stroke", "none");
        circle.setAttribute("data-point", JSON.stringify({ date: day, count, kind }));
        if (!activeKinds.includes(kind)) {
          circle.setAttribute("display", "none");
        }
        pointsGroup.appendChild(circle);
      }
    });
    svg.appendChild(seriesGroup);
    svg.appendChild(pointsGroup);
    // Clear the host and mount the SVG. The placeholder
    // paragraph is removed so it cannot flash through.
    while (host.firstChild) host.removeChild(host.firstChild);
    host.appendChild(svg);
    void wrapper; // wrapper arg reserved for future chart-options binding
  }

  // computeMaxY chooses the y-axis upper bound for the chart.
  // Uses the highest per-day count across every kind + every
  // active series; falls back to 1 so the chart renders an
  // empty archive's "all-zero" state without a div-by-zero.
  /**
   * @param {Record<string, Record<string, number>>} byKind per-kind per-day counts
   * @returns {number} y-axis upper bound
   */
  function computeMaxY(byKind) {
    let max = 0;
    for (const kind of Object.keys(byKind)) {
      const inner = byKind[kind];
      if (!inner || typeof inner !== "object") continue;
      for (const day of Object.keys(inner)) {
        const v = Number(inner[day]) || 0;
        if (v > max) max = v;
      }
    }
    return max === 0 ? 1 : max;
  }

  // chooseYTicks returns a small set of integer y-axis tick
  // values that bracket the chart's max. 1/2/5/10/etc., so the
  // gridlines land on round numbers regardless of magnitude.
  /**
   * @param {number} maxY y-axis upper bound
   * @returns {number[]} round y-axis tick values
   */
  function chooseYTicks(maxY) {
    if (maxY <= 1) return [0, 1];
    if (maxY <= 5) return [0, 1, 2, 3, 4, 5];
    if (maxY <= 10) return [0, 2, 4, 6, 8, 10];
    const step = Math.pow(10, Math.floor(Math.log10(maxY)));
    const ticks = [];
    for (let v = 0; v <= maxY; v += step) {
      ticks.push(v);
      if (ticks.length > 6) break;
    }
    return ticks;
  }

  // linePath emits an SVG path "d" attribute for a single kind.
  // Connects every day in the union-day-list with a straight
  // line; missing days produce a 0-height segment so the path
  // visibly dips rather than skipping.
  /**
   * @param {string} kind storage kind key
   * @param {Record<string, number>} inner per-day counts
   * @param {string[]} days sorted union of days across kinds
   * @param {{top:number,right:number,bottom:number,left:number}} padding chart padding
   * @param {number} innerW chart inner width
   * @param {number} innerH chart inner height
   * @param {number} maxY y-axis upper bound
   * @returns {string} SVG path "d" attribute
   */
  function linePath(kind, inner, days, padding, innerW, innerH, maxY) {
    void kind; // reserved for future per-kind styling variants
    const denom = Math.max(days.length - 1, 1);
    /** @type {string[]} */
    const parts = [];
    days.forEach((day, i) => {
      const count = Number(inner[day]) || 0;
      const x = padding.left + innerW * (i / denom);
      const y = padding.top + innerH * (1 - count / maxY);
      parts.push(`${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`);
    });
    return parts.join(" ");
  }

  // dayIndex returns the position of day in the sorted days
  // array. Linear scan is fine -- the day list is at most a
  // few hundred entries for the rolling archive window.
  /**
   * @param {string} day YYYY-MM-DD key
   * @param {string[]} days sorted day list
   * @returns {number} position of day in days
   */
  function dayIndex(day, days) {
    return days.indexOf(day);
  }

  // kindColor maps each kind to a stroke colour from the locked
  // design-token palette. Saturated enough to read on the
  // parchment background; distinct enough that adjacent lines
  // don't blur into each other.
  /**
   * @param {string} kind storage kind key
   * @returns {string} hex stroke colour
   */
  function kindColor(kind) {
    switch (kind) {
      case "soldier":
        return "#7d4f2d"; // ink / sepia
      case "spouse":
        return "#b6854f"; // warm gold
      case "linked":
        return "#4f7d6b"; // slate green
      case "event":
        return "#a14747"; // review red
      case "article":
        return "#3f5d8a"; // ink blue
      default:
        return "#666";
    }
  }

  // wireInventoryLegend binds click handlers on the legend chips.
  // Toggling flips aria-pressed + the matching <path>'s display,
  // and rewrites the data-active-kinds attribute so a future
  // server-render or htmx swap sees the user's choices. The
  // chips' swatches use the same kindColor palette as the lines.
  /**
   * @param {HTMLElement} wrapper chart wrapper
   * @param {string[]} initialKinds initial active kinds (templ-default)
   */
  function wireInventoryLegend(wrapper, initialKinds) {
    const chips = wrapper.querySelectorAll("[data-inventory-metrics-legend-chip]");
    chips.forEach((chip) => {
      if (!(chip instanceof HTMLElement)) return;
      const kind = chip.getAttribute("data-inventory-metrics-legend-chip");
      if (!kind) return;
      const swatch = chip.querySelector("[data-inventory-metrics-legend-swatch]");
      if (swatch instanceof HTMLElement) {
        swatch.style.backgroundColor = kindColor(kind);
      }
      chip.addEventListener("click", () => {
        const active = readActiveKinds(wrapper);
        const idx = active.indexOf(kind);
        if (idx >= 0) {
          active.splice(idx, 1);
          chip.setAttribute("aria-pressed", "false");
        } else {
          active.push(kind);
          chip.setAttribute("aria-pressed", "true");
        }
        wrapper.setAttribute("data-active-kinds", active.join(","));
        const path = wrapper.querySelector(
          `[data-inventory-metrics-series-kind="${cssEscape(kind)}"]`,
        );
        if (path instanceof SVGElement) {
          if (active.includes(kind)) {
            path.removeAttribute("display");
          } else {
            path.setAttribute("display", "none");
          }
        }
      });
    });
    // initialKinds is read so the chips' aria-pressed state matches
    // the rendered chart on first paint; templ already sets the
    // pressed state but a future refactor could change that.
    void initialKinds;
  }

  // ensureInventoryChartTooltip mounts a single tooltip element
  // inside the chart wrapper. The tooltip is a sibling of the
  // SVG host; positioning is `position: absolute` relative to
  // the wrapper (which is `position: relative` via the inline
  // class the function sets if the wrapper doesn't already have
  // it). The tooltip carries `data-inventory-metrics-tooltip`
  // for the audit probe + a `data-inventory-metrics-tooltip-visible`
  // attribute the hover handler flips. Idempotent: if the tooltip
  // already exists, no-op. The reduced-motion contract is honored
  // via inline CSS (no transition when prefers-reduced-motion:
  // reduce is set).
  /**
   * @param {HTMLElement} wrapper chart wrapper
   */
  function ensureInventoryChartTooltip(wrapper) {
    if (wrapper.querySelector("[data-inventory-metrics-tooltip]")) {
      return;
    }
    const tooltip = document.createElement("div");
    tooltip.setAttribute("data-inventory-metrics-tooltip", "");
    tooltip.setAttribute("data-inventory-metrics-tooltip-visible", "false");
    tooltip.setAttribute("role", "tooltip");
    tooltip.setAttribute("aria-hidden", "true");
    // Parchment-aligned style: subtle border, paper background,
    // small type. Inline so the tooltip does not require a
    // stylesheet edit to render.
    tooltip.style.position = "absolute";
    tooltip.style.pointerEvents = "none";
    tooltip.style.zIndex = "20";
    tooltip.style.padding = "4px 8px";
    tooltip.style.borderRadius = "6px";
    tooltip.style.border = "1px solid rgb(var(--theme-sepia-rgb) / 0.45)";
    tooltip.style.background = "rgba(255, 251, 241, 0.96)";
    tooltip.style.color = "var(--theme-text-primary, #2a1d10)";
    tooltip.style.fontSize = "11px";
    tooltip.style.lineHeight = "1.3";
    tooltip.style.boxShadow = "0 1px 3px rgba(60, 40, 20, 0.18)";
    tooltip.style.opacity = "0";
    tooltip.style.transition = prefersReducedMotion() ? "none" : "opacity 80ms ease-out";
    tooltip.style.whiteSpace = "nowrap";
    wrapper.style.position = "relative";
    wrapper.appendChild(tooltip);
  }

  // wireInventoryChartPoints binds the per-point hover handlers
  // to every <circle data-point> inside the wrapper's SVG.
  // Mouseover reads the circle's data-point attribute, sets
  // the tooltip text + position + visible state. Mouseout
  // hides the tooltip. Mouseleave on the wrapper also hides
  // (covers the cursor exiting the wrapper without crossing
  // a point's mouseout boundary).
  /**
   * @param {HTMLElement} wrapper chart wrapper
   */
  function wireInventoryChartPoints(wrapper) {
    const tooltip = wrapper.querySelector("[data-inventory-metrics-tooltip]");
    if (!(tooltip instanceof HTMLElement)) return;
    const points = wrapper.querySelectorAll("[data-inventory-metrics-points] [data-point]");
    points.forEach((point) => {
      if (!(point instanceof Element)) return;
      point.addEventListener("mouseover", (event) => {
        const raw = point.getAttribute("data-point") || "{}";
        let payload;
        try {
          payload = JSON.parse(raw);
        } catch {
          return;
        }
        if (!payload || typeof payload !== "object") return;
        const label = inventoryMetricsKindLabel(payload.kind);
        tooltip.textContent = `${payload.date} · ${payload.count} · ${label}`;
        tooltip.setAttribute("data-inventory-metrics-tooltip-visible", "true");
        tooltip.setAttribute("aria-hidden", "false");
        tooltip.style.opacity = "1";
        // Position relative to the wrapper.
        if (event instanceof MouseEvent) {
          const wrapperRect = wrapper.getBoundingClientRect();
          const x = event.clientX - wrapperRect.left + 8;
          const y = event.clientY - wrapperRect.top + 8;
          tooltip.style.left = clamp(x, 0, wrapperRect.width - tooltip.offsetWidth - 4) + "px";
          tooltip.style.top = clamp(y, 0, wrapperRect.height - tooltip.offsetHeight - 4) + "px";
        }
      });
      point.addEventListener("mouseout", () => {
        hideInventoryChartTooltip(tooltip);
      });
    });
    wrapper.addEventListener("mouseleave", () => {
      hideInventoryChartTooltip(tooltip);
    });
  }

  // hideInventoryChartTooltip flips the visible + aria-hidden
  // attributes and fades the opacity. Kept tiny so the
  // mouseout + mouseleave paths share one implementation.
  /**
   * @param {HTMLElement} tooltip tooltip element
   */
  function hideInventoryChartTooltip(tooltip) {
    tooltip.setAttribute("data-inventory-metrics-tooltip-visible", "false");
    tooltip.setAttribute("aria-hidden", "true");
    tooltip.style.opacity = "0";
  }

  // clamp constrains a value to [min, max]. Used to keep the
  // tooltip inside the chart wrapper bounds so it never escapes
  // the visible area on edge-of-chart hovers.
  /**
   * @param {number} n value
   * @param {number} min lower bound
   * @param {number} max upper bound
   * @returns {number} clamped value
   */
  function clamp(n, min, max) {
    if (Number.isNaN(n)) return min;
    if (n < min) return min;
    if (n > max) return max;
    return n;
  }

  // prefersReducedMotion returns true when the user has
  // requested reduced motion. Used by the tooltip's transition
  // to disable the fade-in animation.
  /**
   * @returns {boolean} true if reduced motion is preferred
   */
  function prefersReducedMotion() {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return false;
    }
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  // inventoryMetricsKindLabel mirrors the templ-rendered label
  // for each kind key. The chart's hover tooltip needs the
  // human-readable label (not the storage key) so this small
  // mirror lives next to the hover handler rather than a
  // cross-module import.
  /**
   * @param {string} kind storage kind key
   * @returns {string} human-readable label
   */
  function inventoryMetricsKindLabel(kind) {
    switch (kind) {
      case "soldier":
        return "Soldiers";
      case "spouse":
        return "Spouse Records";
      case "linked":
        return "Linked Persons";
      case "event":
        return "Event Records";
      case "article":
        return "Articles";
      default:
        return kind;
    }
  }

  // cssEscape escapes an arbitrary string into a CSS attribute
  // selector. SVG <path> data-inventory-metrics-series-kind
  // values come from the templ-rendered kind keys so they are
  // always one of the locked 5 strings, but defensive escaping
  // is the right discipline for a querySelector argument.
  /**
   * @param {unknown} value arbitrary string
   * @returns {string} CSS-attribute-selector-safe form
   */
  function cssEscape(value) {
    const s = typeof value === "string" ? value : String(value);
    if (typeof window !== "undefined" && typeof window.CSS !== "undefined" && typeof window.CSS.escape === "function") {
      return window.CSS.escape(s);
    }
    return s.replace(/[^a-zA-Z0-9_-]/g, "\\$&");
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

  // installFoldouts wires every element with [data-foldout-trigger]
  // to its [data-foldout-panel] (matched by the same data-* value).
  // The Share top-nav foldout is the first consumer (issue #264);
  // any future nav item that wants the same affordance just adds
  // a trigger + panel pair with matching data-foldout-* values and
  // this init picks them up uniformly.
  //
  // The contract:
  //   - Click the trigger → toggle the panel; aria-expanded mirrors
  //     the open state.
  //   - Click anywhere outside the trigger or the panel → close.
  //   - ESC → close + return focus to the trigger.
  //   - Tab / Shift+Tab inside the panel → move through menuitems in
  //     source order (the browser default — we don't intercept Tab
  //     so focus naturally cycles to the next focusable element).
  //   - ArrowDown / ArrowUp on the trigger or inside the panel →
  //     move focus to the next / previous menuitem. Home / End jump
  //     to first / last. Enter is the browser default on <a>.
  //   - When window.location.pathname starts with the trigger's
  //     aria-controls stem, the trigger gets aria-current="page".
  //     Per the issue's locked decision, ONLY the trigger gets the
  //     active indicator — sub-items stay plain.
  function installFoldouts() {
    // Idempotency guard. installFoldouts attaches one
    // document-level click listener for outside-click-to-close
    // (one per call). Without this guard, the function would
    // compound listeners on every htmx swap and a single
    // outside click would close the panel N times. We need
    // htmx-swap re-runs so the document handler stays bound
    // after the body is swapped AND so per-trigger handlers
    // get re-attached to the new trigger DOM nodes that htmx
    // inserts (old nodes are detached and lose their closures).
    // The guard keeps the document handler bound once; the
    // per-trigger loop below re-attaches on every call to pick
    // up newly-rendered triggers. See issue #285 + handoff:
    // .rdivide/handoff-foldout-click-race.md.
    if (!window.__foldoutInstallN) window.__foldoutInstallN = 0;
    window.__foldoutInstallN++;
    // Expose a re-init handle for the issue #285 regression
    // probe. The handle is attached unconditionally so the
    // smoke test can force a re-install (the same path
    // htmx:load takes) and assert idempotency end-to-end.
    // The cost is one function reference on window; the
    // function itself is the same one htmx:load calls
    // indirectly via initializeDynamicContent.
    if (!window.__foldoutProbeReinit) {
      window.__foldoutProbeReinit = () => installFoldouts();
    }
    if (!window.__foldoutDocHandlerBound) {
      window.__foldoutDocHandlerBound = true;
      document.addEventListener("click", (event) => {
        const target = event.target;
        if (!(target instanceof Node)) {
          return;
        }
        for (const trigger of document.querySelectorAll("[data-foldout-trigger]")) {
          if (!(trigger instanceof HTMLElement)) continue;
          // Skip the panel whose trigger owns the click. The
          // click on the trigger button has target === trigger;
          // without this guard, the panel that was JUST opened
          // by the trigger's own click would be immediately
          // closed by the outside-click handler. See commit
          // d8f73b7 for the original race fix.
          if (trigger === target || trigger.contains(target)) continue;
          /** @type {string | null} */
          const id = trigger.getAttribute("data-foldout-trigger");
          if (!id) continue;
          /** @type {Element | null} */
          const panel = document.querySelector('[data-foldout-panel="' + id + '"]');
          if (!(panel instanceof HTMLElement)) continue;
          if (panel.classList.contains("hidden")) continue;
          if (panel.contains(target)) continue;
          panel.classList.add("hidden");
          trigger.setAttribute("aria-expanded", "false");
        }
      });
    }
    const triggers = document.querySelectorAll("[data-foldout-trigger]");
    // Per-trigger idempotency: re-running installFoldouts
    // (e.g. on htmx:load) must not double-attach click
    // handlers to a trigger. Doubled handlers would call
    // toggle() twice per click — open() then close() — and
    // the panel would flash open and immediately close.
    // The cold-start Wails bug (#285) was traced to a
    // missing re-init; the smoke regression that would
    // catch a double-bind is here. See handoff:
    // .rdivide/handoff-foldout-click-race.md.
    if (!window.__foldoutBoundTriggers) {
      window.__foldoutBoundTriggers = new WeakSet();
    }
    for (const trigger of triggers) {
      if (!(trigger instanceof HTMLElement)) {
        continue;
      }
      const menuID = trigger.getAttribute("data-foldout-trigger");
      if (!menuID) {
        continue;
      }
      // Skip triggers that already had their per-trigger
      // listeners attached. installFoldouts is meant to be
      // re-runnable so newly-rendered triggers (after an
      // htmx swap) get wired; the existing ones must not be
      // re-wired or toggle() would fire twice per click.
      if (window.__foldoutBoundTriggers.has(trigger)) {
        continue;
      }
      const panel = document.querySelector('[data-foldout-panel="' + menuID + '"]');
      if (!(panel instanceof HTMLElement)) {
        continue;
      }
      window.__foldoutBoundTriggers.add(trigger);
      // Single shared click-outside handler closes all open panels
      // so a click that opens a different foldout cleanly closes
      // the first one without two handlers racing.
      const isOpen = () => !panel.classList.contains("hidden");
      const open = () => {
        // Close any sibling foldouts first.
        for (const t of document.querySelectorAll("[data-foldout-trigger]")) {
          if (t === trigger) continue;
          const id = t.getAttribute("data-foldout-trigger");
          if (!id) continue;
          const p = document.querySelector('[data-foldout-panel="' + id + '"]');
          if (p instanceof HTMLElement) p.classList.add("hidden");
          t.setAttribute("aria-expanded", "false");
        }
        panel.classList.remove("hidden");
        trigger.setAttribute("aria-expanded", "true");
        // Issue #476: measure the trigger + panel and shift the panel
        // so it stays fully inside the viewport. Without this the
        // panel can extend leftward past the viewport on narrow
        // windows where the trigger sits near the right edge.
        placePopoutPanel(trigger, panel);
        // Move focus to the first menuitem so keyboard users land
        // inside the panel after Enter/Space on the trigger.
        const firstItem = panel.querySelector('[role="menuitem"]');
        if (firstItem instanceof HTMLElement) {
          firstItem.focus();
        }
      };
      /** @param {boolean} returnFocus */
      const close = (returnFocus) => {
        panel.classList.add("hidden");
        trigger.setAttribute("aria-expanded", "false");
        // Issue #476: clear the placement transform when closing so
        // the next open() measures from the panel's natural anchored
        // position, not from a stale transform.
        panel.style.removeProperty("transform");
        if (returnFocus && document.activeElement === panel) {
          trigger.focus();
        } else if (returnFocus) {
          // ESC path: always return focus to the trigger.
          trigger.focus();
        }
      };
      const toggle = () => {
        if (isOpen()) {
          close(false);
        } else {
          open();
        }
      };
      trigger.addEventListener("click", (event) => {
        event.preventDefault();
        toggle();
      });
      // Keyboard nav: ArrowDown opens + moves to first item;
      // ArrowUp opens + moves to last item. ESC closes.
      trigger.addEventListener("keydown", (event) => {
        if (event.key === "ArrowDown") {
          event.preventDefault();
          if (!isOpen()) open();
          else focusSibling(panel, "next");
        } else if (event.key === "ArrowUp") {
          event.preventDefault();
          if (!isOpen()) open();
          else focusSibling(panel, "prev");
        } else if (event.key === "Escape" && isOpen()) {
          event.preventDefault();
          close(true);
        }
      });
      panel.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          close(true);
        } else if (event.key === "ArrowDown") {
          event.preventDefault();
          focusSibling(panel, "next");
        } else if (event.key === "ArrowUp") {
          event.preventDefault();
          focusSibling(panel, "prev");
        } else if (event.key === "Home") {
          event.preventDefault();
          const first = panel.querySelector('[role="menuitem"]');
          if (first instanceof HTMLElement) first.focus();
        } else if (event.key === "End") {
          event.preventDefault();
          const items = panel.querySelectorAll('[role="menuitem"]');
          const last = items[items.length - 1];
          if (last instanceof HTMLElement) last.focus();
        }
      });
      // Auto-close on sub-item click. The browser default
      // navigates to the link; the next page load replaces
      // the panel DOM so the hidden state is irrelevant, but
      // closing immediately prevents a flash of an-open
      // panel during the navigation transition.
      for (const item of panel.querySelectorAll('[role="menuitem"]')) {
        item.addEventListener("click", () => {
          close(false);
        });
      }
      // Active-page indicator. If the current path starts with
      // /share, the Share trigger is the active page. Future
      // triggers that adopt this pattern should add their own
      // mapping (Browse → /browse*, etc.).
      const stem = stemForTrigger(menuID);
      if (stem && window.location.pathname.indexOf(stem) === 0) {
        trigger.setAttribute("aria-current", "page");
      }
    }
  }

  // installMegaMenus wires every element with
  // [data-mega-menu-trigger] to its [data-mega-menu-panel]
  // (matched by the same data-* value). Issue #380 — the
  // Records + Share & Review mega-menus use this selector pair
  // (parallel to [data-foldout-*] used by the flat Share +
  // Research & Review foldouts). The contract matches
  // installFoldouts: click toggles, outside-click closes,
  // aria-expanded mirrors open state. Arrow-key navigation
  // falls through to the browser default (Tab cycles through
  // the menuitems inside the panel in source order) — the
  // primitive is a 2D grid, so arrow-key behavior matches the
  // user's mental model of moving through a table.
  function installMegaMenus() {
    if (!window.__megaMenuInstallN) window.__megaMenuInstallN = 0;
    window.__megaMenuInstallN++;

    if (!window.__megaMenuDocHandlerBound) {
      window.__megaMenuDocHandlerBound = true;
      document.addEventListener("click", (event) => {
        const target = event.target;
        if (!(target instanceof Node)) return;
        for (const trigger of document.querySelectorAll("[data-mega-menu-trigger]")) {
          if (!(trigger instanceof HTMLElement)) continue;
          if (trigger === target || trigger.contains(target)) continue;
          /** @type {string | null} */
          const id = trigger.getAttribute("data-mega-menu-trigger");
          if (!id) continue;
          /** @type {Element | null} */
          const panel = document.querySelector('[data-mega-menu-panel="' + id + '"]');
          if (!(panel instanceof HTMLElement)) continue;
          if (panel.classList.contains("hidden")) continue;
          if (panel.contains(target)) continue;
          panel.classList.add("hidden");
          trigger.setAttribute("aria-expanded", "false");
        }
      });
    }

    if (!window.__megaMenuBoundTriggers) {
      window.__megaMenuBoundTriggers = new WeakSet();
    }
    const triggers = document.querySelectorAll("[data-mega-menu-trigger]");
    for (const trigger of triggers) {
      if (!(trigger instanceof HTMLElement)) continue;
      if (window.__megaMenuBoundTriggers.has(trigger)) continue;
      const menuID = trigger.getAttribute("data-mega-menu-trigger");
      if (!menuID) continue;
      const panel = document.querySelector('[data-mega-menu-panel="' + menuID + '"]');
      if (!(panel instanceof HTMLElement)) continue;
      window.__megaMenuBoundTriggers.add(trigger);

      const isOpen = () => !panel.classList.contains("hidden");
      const open = () => {
        for (const t of document.querySelectorAll("[data-mega-menu-trigger]")) {
          if (t === trigger) continue;
          const id = t.getAttribute("data-mega-menu-trigger");
          if (!id) continue;
          const p = document.querySelector('[data-mega-menu-panel="' + id + '"]');
          if (p instanceof HTMLElement) p.classList.add("hidden");
          t.setAttribute("aria-expanded", "false");
        }
        panel.classList.remove("hidden");
        trigger.setAttribute("aria-expanded", "true");
        // Issue #476: measure + shift the panel so it stays inside
        // the viewport on narrow windows. Same helper as foldouts.
        placePopoutPanel(trigger, panel);
        const firstItem = panel.querySelector('[role="menuitem"]');
        if (firstItem instanceof HTMLElement) firstItem.focus();
      };
      /** @param {boolean} returnFocus */
      const close = (returnFocus) => {
        panel.classList.add("hidden");
        trigger.setAttribute("aria-expanded", "false");
        // Issue #476: clear the placement transform when closing.
        panel.style.removeProperty("transform");
        if (returnFocus) trigger.focus();
      };
      const toggle = () => { isOpen() ? close(false) : open(); };
      trigger.addEventListener("click", (event) => {
        event.preventDefault();
        toggle();
      });
      trigger.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          close(true);
        }
      });
      panel.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          close(true);
        }
      });
    }
  }

  // Issue #476: installFloatingNavPanel binds the
  // [data-floating-nav-toggle] Menu button in the floating dock to
  // the [data-floating-nav-panel] Quick Navigation popout. Pre-#476
  // the toggle used a templ inline onclick that toggled the hidden
  // class but never invoked any placement helper, so on a narrow
  // window the panel (anchored to right-4 sm:right-6) could clip
  // the left edge if the panel was wider than the available space.
  // placePopoutPanel measures the trigger + panel and applies a
  // translate3d so the panel stays inside the viewport.
  function installFloatingNavPanel() {
    if (!window.__floatingNavInstallN) window.__floatingNavInstallN = 0;
    window.__floatingNavInstallN++;
    const triggers = document.querySelectorAll("[data-floating-nav-toggle]");
    if (!window.__floatingNavBoundTriggers) {
      window.__floatingNavBoundTriggers = new WeakSet();
    }
    for (const trigger of triggers) {
      if (!(trigger instanceof HTMLElement)) continue;
      if (window.__floatingNavBoundTriggers.has(trigger)) continue;
      const panel = document.querySelector("[data-floating-nav-panel]");
      if (!(panel instanceof HTMLElement)) continue;
      window.__floatingNavBoundTriggers.add(trigger);
      const isOpen = () => !panel.classList.contains("hidden");
      const open = () => {
        panel.classList.remove("hidden");
        // Issue #476: smart-placement so the panel doesn't overflow
        // the left edge on narrow windows.
        placePopoutPanel(trigger, panel);
        const firstItem = panel.querySelector('[role="menuitem"], a[href]');
        if (firstItem instanceof HTMLElement) firstItem.focus();
      };
      /** @param {boolean} returnFocus */
      const close = (returnFocus) => {
        panel.classList.add("hidden");
        // Issue #476: clear the placement transform when closing.
        panel.style.removeProperty("transform");
        if (returnFocus) trigger.focus();
      };
      const toggle = () => { isOpen() ? close(false) : open(); };
      trigger.addEventListener("click", (event) => {
        event.preventDefault();
        toggle();
      });
      trigger.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && isOpen()) {
          event.preventDefault();
          close(true);
        }
      });
      panel.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          close(true);
        }
      });
    }
  }

  // focusSibling moves focus to the next/previous menuitem in the
  // panel relative to the currently-focused element. Wraps around
  // so the user can keep pressing ArrowDown to cycle through the
  // list. The WAI-ARIA menu pattern spec wraps; we follow it.
  /**
   * @param {HTMLElement} panel
   * @param {"next" | "prev"} direction
   */
  function focusSibling(panel, direction) {
    const items = Array.from(panel.querySelectorAll('[role="menuitem"]'));
    if (items.length === 0) {
      return;
    }
    const current = document.activeElement;
    const idx = items.indexOf(/** @type {Element} */ (current));
    let next;
    if (idx === -1) {
      next = direction === "next" ? 0 : items.length - 1;
    } else {
      next = direction === "next"
        ? (idx + 1) % items.length
        : (idx - 1 + items.length) % items.length;
    }
    const nextTarget = items[next];
    if (nextTarget instanceof HTMLElement) {
      nextTarget.focus();
    }
  }

  // stemForTrigger maps a foldout menu id to the URL path stem
  // that should set aria-current=page on the trigger. The
  // current contract: menu id "layout.share.menu" → "/share".
  // Future triggers add their own mapping. Keeping this in one
  // place makes it easy to audit which nav items are "active"
  // on which routes.
  /** @param {string} menuID */
  function stemForTrigger(menuID) {
    if (menuID === "layout.share.menu") return "/share";
    return null;
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
      if (!(form instanceof HTMLFormElement) || !(countNode instanceof HTMLElement)) {
        return;
      }
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

  /**
   * @param {Element} section
   * @param {boolean} enabled
   */
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

  /** @param {HTMLFormElement | Element} form */
  function syncEntryTypeFields(form) {
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const select = form.querySelector("[data-entry-type-select]");
    if (!(select instanceof HTMLSelectElement)) {
      return;
    }
    // Event Records are authored via /events/new, not /soldiers/new
    // (issue #362), so this dispatcher no longer carries any
    // data-event-only-field gate or the form-action swap to /events/new.
    const specialEntry = select.value === "wife" || select.value === "widow" || select.value === "linked_person";
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
    if (form.dataset.entryTypeFormAction !== "/soldiers") {
      form.action = "/soldiers";
      form.dataset.entryTypeFormAction = "/soldiers";
    }
  }

  /** @param {string} value */
  function isSoldierEntryType(value) {
    // True for the default Soldier subtype and the linked-person
    // subtypes (wife / widow / linked_person) — anything that
    // owns soldier-shaped fields like rank, unit, or service dates.
    // Event Records are authored via /events/new only (issue #362),
    // so the helper never sees an "event" value here.
    return value !== "wife" && value !== "widow" && value !== "linked_person";
  }

  function initializeEntryTypeForms() {
    document.querySelectorAll("form").forEach((form) => {
      syncEntryTypeFields(form);
    });
  }

  /** @param {HTMLInputElement | HTMLTextAreaElement} input */
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

  /**
   * @param {HTMLFormElement} form
   */
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

  /**
   * @param {string} path
   */
  function normalizedRedirectPath(path) {
    if (path === "/export") {
      return "/share";
    }
    return path;
  }

  /**
   * @param {unknown} state
   */
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

  /**
   * @param {unknown} state
   */
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

  /**
   * @param {string} message
   * @param {string} [kind]
   */
  function showToast(message, kind = "success") {
    const region = toastRegion();
    if (!(region instanceof HTMLElement) || !message) {
      return;
    }
    /** @type {Record<string, string>} */
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

  /**
   * @param {HTMLElement} el
   * @param {string} redirectTo
   * @param {string} responseText
   * @param {{scrollX?: number, scrollY?: number} | undefined} requestState
   */
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

  /**
   * @param {string | number} monthValue
   */
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
    rememberResearchPickFromPage();
    hydrateRecentSearchResults();
    hydrateResearchPickerRecents();
    initializeBrowseView();
    applyCalendarAnniversaryDensity();
    initializeCopyPathButtons();
    initializeInventoryMetricsChart();
    initializePersonRecordPicker();
    initializeMarkdownCheatsheet();
    // installFoldouts is idempotent (guarded by
    // window.__foldoutDocHandlerBound) so calling it here on
    // every htmx:load is safe. The per-trigger loop inside
    // re-attaches click listeners to newly-rendered trigger
    // elements, which is required because the original DOM
    // node from cold-start installFoldouts may be detached
    // by a subsequent htmx swap. Without this, the cold-start
    // install ran once on "/" with triggerCount: 0 (no
    // layout nav in the response), and every later trigger
    // rendered by htmx had no listener until the user
    // navigated twice. See issue #285 + handoff:
    // .rdivide/handoff-foldout-click-race.md.
    installFoldouts();
    installMegaMenus();
    installFloatingNavPanel();
    installTermDisclosures();
    installAboutActivityHeatmap();
    document.querySelectorAll("form[data-pdf-pref-scope]").forEach((form) => applyPDFPreferences(form));
  }

  // installAboutActivityHeatmap (issue #601) reads the
  // per-day counts JSON the templ partial emits on
  // [data-about-activity-heatmap-data], paints a 52-week
  // x 7-day SVG grid into the host, and removes the
  // "Loading heatmap..." placeholder paragraph.
  //
  // The render is idempotent across htmx:load re-renders
  // via the `__aboutHeatmapPainted` guard — the JS skips
  // hosts that have already been painted.
  //
  // Layout: 52 columns (oldest on the left) x 7 rows
  // (Sun-Sat) so the grid reads like a calendar. Each
  // day is a small rounded <rect> colored by
  // count/rolling-max on a 5-stop sepia -> amber ramp.
  // Empty days get the lowest tint so the user can
  // visually scan where activity happened. The whole
  // SVG carries role="img" + aria-label naming the
  // heatmap's content so screen readers announce the
  // heatmap once on land instead of per-cell.
  //
  // No tooltip in v1 — the issue body explicitly defers
  // hover/tooltip behavior to a follow-up. Empty-state
  // grid (no data): renders the all-empty grid silently.
  function installAboutActivityHeatmap() {
    const hosts = document.querySelectorAll("[data-about-activity-heatmap]");
    hosts.forEach((host) => {
      if (!(host instanceof HTMLElement)) return;
      if (host.__aboutHeatmapPainted === true) return;
      const raw = host.getAttribute("data-about-activity-heatmap-data");
      if (!raw) return;
      /** @type {Record<string, number>} */
      let byDay = {};
      try {
        byDay = JSON.parse(raw);
        if (!byDay || typeof byDay !== "object") {
          byDay = {};
        }
      } catch (err) {
        if (typeof console !== "undefined") {
          console.warn("about activity heatmap: invalid JSON in data-about-activity-heatmap-data", err);
        }
        byDay = {};
      }
      paintAboutActivityHeatmap(/** @type {HTMLElement} */ (host), byDay);
      host.__aboutHeatmapPainted = true;
    });
  }

  // paintAboutActivityHeatmap (issue #601) writes the
  // SVG grid into the host and removes the loading
  // placeholder. The host's children are cleared first
  // so the templ placeholder + any leftover artifacts
  // from a prior abortive paint cannot leak through.
  //
  // 5-stop ramp: 0 -> faint sepia tint, 1 -> warm
  // sepia, max/2 -> amber, max -> deep gold. The exact
  // CSS-var tokens match the existing chart palette so
  // the visual language stays consistent across surfaces.
  /**
   * @param {HTMLElement} host SVG mount host
   * @param {Record<string, number>} byDay per-day commit counts
   */
  // SVG NS shorthand for the cells + labels below.
  const SVG_NS = "http://www.w3.org/2000/svg";
  // Month labels for the heatmap axis (3-letter
  // abbreviations, GitHub/GitLab convention).
  const HEATMAP_MONTH_LABELS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  // Weekday labels for the heatmap left-margin gutter
  // (1-letter abbreviations; only Sun / Wed / Fri render
  // to avoid overcrowding the column).
  const HEATMAP_WEEKDAY_LABELS = ["S", "M", "T", "W", "T", "F", "S"];

  /**
   * @param {HTMLElement} host SVG mount host
   * @param {Record<string, number>} byDay per-day commit counts
   */
  function paintAboutActivityHeatmap(host, byDay) {
    // Compute the rolling max count across the visible
    // window. byDay is `date(YYYY-MM-DD) -> count`;
    // max-of-keys for the ramp stops.
    let maxCount = 0;
    for (const k of Object.keys(byDay)) {
      const v = byDay[k];
      if (typeof v === "number" && v > maxCount) maxCount = v;
    }
    // SVG layout: 52 cols, 7 rows (Sun-Sat). Cell size
    // 12px wide, with 2px gap. Total grid width 720px
    // (52*12 + 51*2). Total height 7*12 + 6*2 = 96px.
    //
    // Issue #602 axes: 24px left gutter for weekday
    // labels + 24px top gutter for month labels.
    // Updated dimensions: width 744px (720 + 24), height
    // 120px (96 + 24). The grid offsets by (24, 24)
    // so cells render in the bottom-right 720x96 region.
    const cell = 12;
    const gap = 2;
    const cols = 52;
    const rows = 7;
    const gridWidth = cols * cell + (cols - 1) * gap;
    const gridHeight = rows * cell + (rows - 1) * gap;
    const gutterX = 24;
    const gutterY = 24;
    const width = gridWidth + gutterX;
    const height = gridHeight + gutterY;
    // Empty all host children (removes the loading
    // placeholder + any stray leftovers).
    while (host.firstChild) host.removeChild(host.firstChild);
    const svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("viewBox", "0 0 " + width + " " + height);
    svg.setAttribute("width", String(width));
    svg.setAttribute("height", String(height));
    svg.setAttribute("role", "img");
    svg.setAttribute("aria-label", "Repository activity heatmap: " + Object.keys(byDay).length + " active days, max " + maxCount + " commits per day");
    svg.classList.add("about-activity-heatmap-svg");
    // Find the most recent date in byDay; the heatmap
    // renders the 52 weeks ending at that date so the
    // "today" column is on the right.
    let endDate = new Date();
    const dateKeys = Object.keys(byDay);
    if (dateKeys.length > 0) {
      const latest = dateKeys.sort().slice(-1)[0];
      const parts = latest.split("-").map((p) => parseInt(p, 10));
      if (parts.length === 3 && parts.every((p) => Number.isFinite(p))) {
        endDate = new Date(Date.UTC(parts[0], parts[1] - 1, parts[2]));
      }
    }
    /** @type {Record<string, number>} */
    const firstColumnOfMonth = {};
    for (let col = 0; col < cols; col++) {
      for (let row = 0; row < rows; row++) {
        const dayOffset = -(cols - 1 - col) * 7 + row;
        const cellDate = new Date(endDate.getTime() + dayOffset * 86400000);
        if (isNaN(cellDate.getTime())) continue;
        const y = cellDate.getUTCFullYear();
        const m = String(cellDate.getUTCMonth() + 1).padStart(2, "0");
        const d = String(cellDate.getUTCDate()).padStart(2, "0");
        const dateKey = y + "-" + m + "-" + d;
        const monthKey = y + "-" + m;
        if (firstColumnOfMonth[monthKey] === undefined) {
          firstColumnOfMonth[monthKey] = col;
        }
        const count = byDay[dateKey] || 0;
        const rect = document.createElementNS(SVG_NS, "rect");
        rect.setAttribute("x", String(gutterX + col * (cell + gap)));
        rect.setAttribute("y", String(gutterY + row * (cell + gap)));
        rect.setAttribute("width", String(cell));
        rect.setAttribute("height", String(cell));
        rect.setAttribute("rx", "2");
        rect.setAttribute("fill", aboutActivityHeatmapFill(count, maxCount));
        rect.setAttribute("data-about-activity-heatmap-cell", dateKey);
        rect.setAttribute("data-about-activity-heatmap-count", String(count));
        rect.setAttribute("tabindex", "0");
        rect.setAttribute("role", "img");
        // Singular / plural matches the user's mental
        // model ("1 commit" vs "3 commits"); native
        // <title> shows it on hover + focus.
        const labelSingularSuffix = count === 1 ? "" : "s";
        rect.setAttribute("aria-label", dateKey + " · " + count + " commit" + labelSingularSuffix);
        const title = document.createElementNS(SVG_NS, "title");
        title.textContent = dateKey + " · " + count + " commit" + labelSingularSuffix;
        rect.appendChild(title);
        svg.appendChild(rect);
      }
    }
    // Month labels above the grid (first column of each
    // new month). Sorted by month key so render order
    // matches the calendar axis.
    const monthLabelPositions = Object.keys(firstColumnOfMonth).sort();
    for (const mk of monthLabelPositions) {
      const col = firstColumnOfMonth[mk];
      const monthIdx = parseInt(mk.split("-")[1], 10) - 1;
      const text = document.createElementNS(SVG_NS, "text");
      text.setAttribute("x", String(gutterX + col * (cell + gap)));
      text.setAttribute("y", String(gutterY - 8));
      text.setAttribute("font-size", "10");
      text.setAttribute("fill", "var(--theme-text-mid, #6b6b6b)");
      text.setAttribute("font-family", "system-ui, sans-serif");
      text.setAttribute("data-about-activity-heatmap-month", HEATMAP_MONTH_LABELS[monthIdx]);
      text.textContent = HEATMAP_MONTH_LABELS[monthIdx];
      svg.appendChild(text);
    }
    // Weekday labels in the left margin: only Sun / Wed
    // / Fri to keep the column visually balanced.
    for (let row = 0; row < rows; row++) {
      if (row !== 0 && row !== 2 && row !== 4) continue;
      const text = document.createElementNS(SVG_NS, "text");
      text.setAttribute("x", String(gutterX - 6));
      text.setAttribute("y", String(gutterY + row * (cell + gap) + cell - 3));
      text.setAttribute("font-size", "10");
      text.setAttribute("fill", "var(--theme-text-mid, #6b6b6b)");
      text.setAttribute("font-family", "system-ui, sans-serif");
      text.setAttribute("text-anchor", "end");
      text.setAttribute("data-about-activity-heatmap-weekday", HEATMAP_WEEKDAY_LABELS[row]);
      text.textContent = HEATMAP_WEEKDAY_LABELS[row];
      svg.appendChild(text);
    }
    host.appendChild(svg);
  }

  // aboutActivityHeatmapFill maps a day count to a 5-stop
  // ramp color. Empty (0 count) gets the faintest tint so
  // the empty cells visually recede; active days scale
  // toward warm gold.
  /**
   * @param {number} count commits per day
   * @param {number} maxCount rolling max across all days
   * @returns {string} CSS color value
   */
  function aboutActivityHeatmapFill(count, maxCount) {
    if (count <= 0) return "rgba(125, 79, 45, 0.08)"; // faint sepia
    if (maxCount <= 0) return "rgba(125, 79, 45, 0.08)";
    const ratio = Math.min(count / maxCount, 1);
    if (ratio <= 0.25) return "rgba(125, 79, 45, 0.30)"; // warm sepia
    if (ratio <= 0.50) return "rgba(151, 105, 60, 0.55)"; // amber-mid
    if (ratio <= 0.75) return "rgba(183, 133, 79, 0.78)"; // gold
    return "#b6854f"; // peak gold
  }


  // installTermDisclosures attaches click + Escape +
  // outside-click handlers to every [data-term-disclosure-trigger]
  // element (issue #564 slice 2).
  //
  // Each trigger is a <button type="button"> rendered by
  // `internal/templates/components/term_disclosure.templ`. The
  // popover panel carries `data-term-disclosure-panel="<slug>"`
  // and starts hidden (the templ sets the `hidden` attribute).
  // On click, Enter, or Space the JS removes `hidden`,
  // mirrors aria-expanded="true", and focuses the panel's
  // "Read in glossary" link so the next Enter navigates
  // straight to the /about kebab anchor. On Escape or
  // outside click the JS restores hidden + aria-expanded="false"
  // + restores focus to the trigger.
  //
  // Idempotent: every call re-runs the per-trigger loop and
  // re-attaches the document-level outside-click handler
  // (deduped by window.__termDisclosureDocHandlerBound, the
  // installFoldouts-equivalent of the codebase pattern). htmx
  // swaps that re-render triggers get fresh listeners without
  // the user seeing a stuck-open popover.
  function installTermDisclosures() {
    /** @type {NodeListOf<HTMLElement>} */
    const triggers = document.querySelectorAll("[data-term-disclosure-trigger]");
    triggers.forEach((trigger) => {
      const slug = trigger.getAttribute("data-term-disclosure-trigger");
      if (!slug) return;
      const panel = document.getElementById("term-disclosure-panel-" + slug);
      if (!panel) return;
      // Skip if we already wired this trigger (defensive: a
      // second installTermDisclosures call from htmx:load
      // re-attaches the click listener, but the outside-click
      // handler must dedupe).
      if (trigger.dataset.termDisclosureWired === "1") {
        // Reopen state if the trigger is already-open from
        // a prior interaction that survived an htmx swap.
        return;
      }
      trigger.dataset.termDisclosureWired = "1";
      const open = () => {
        // Close every other open disclosure first so only
        // one popover is visible at a time. Mirrors the
        // foldout "single panel open" convention.
        /** @type {NodeListOf<HTMLElement>} */
        const otherPanels = document.querySelectorAll("[data-term-disclosure-panel]:not([hidden])");
        otherPanels.forEach((el) => {
          if (el.id !== panel.id) {
            el.hidden = true;
            const otherSlug = el.getAttribute("data-term-disclosure-panel");
            const otherTrigger = /** @type {HTMLElement|null} */ (document.querySelector(
              "[data-term-disclosure-trigger='" + otherSlug + "']",
            ));
            if (otherTrigger) otherTrigger.setAttribute("aria-expanded", "false");
          }
        });
        panel.hidden = false;
        trigger.setAttribute("aria-expanded", "true");
        // Move focus into the popover so screen-reader users
        // hear the disclosure body. The "Read in glossary"
        // link is the natural focus target.
        const readLink = /** @type {HTMLElement|null} */ (panel.querySelector("[data-term-disclosure-read-in-glossary]"));
        if (readLink) readLink.focus();
      };
      const close = () => {
        panel.hidden = true;
        trigger.setAttribute("aria-expanded", "false");
        trigger.focus();
      };
      trigger.addEventListener("click", (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (panel.hidden) {
          open();
        } else {
          close();
        }
      });
      // Enter / Space on the trigger element fire a click
      // automatically (the <button type="button"> default),
      // so the listener above covers them. We do not need a
      // separate keydown listener — relying on the browser
      // default here keeps the JS small.
    });

    // Document-level outside-click + Escape handlers. Bound
    // once per DOMContentLoaded install; subsequent
    // installTermDisclosures calls skip the bind via the
    // __termDisclosureDocHandlerBound dedupe (mirrors
    // installFoldouts' __foldoutDocHandlerBound).
    if (!window.__termDisclosureDocHandlerBound) {
      window.__termDisclosureDocHandlerBound = true;
      document.addEventListener("click", (e) => {
        const openPanel = /** @type {HTMLElement|null} */ (document.querySelector(
          "[data-term-disclosure-panel]:not([hidden])",
        ));
        if (!openPanel) return;
        const slug = openPanel.getAttribute("data-term-disclosure-panel");
        const trigger = /** @type {HTMLElement|null} */ (document.querySelector(
          "[data-term-disclosure-trigger='" + slug + "']",
        ));
        if (!trigger) return;
        // Click on the trigger itself is handled by the
        // per-trigger listener (which close()s before
        // re-opening), so we only act on clicks elsewhere
        // in the document.
        const target = /** @type {Node|null} */ (e.target);
        if (target && trigger.contains(target)) return;
        if (target && openPanel.contains(target)) return;
        openPanel.hidden = true;
        trigger.setAttribute("aria-expanded", "false");
      });
      document.addEventListener("keydown", (e) => {
        if (e.key !== "Escape") return;
        const openPanel = /** @type {HTMLElement|null} */ (document.querySelector(
          "[data-term-disclosure-panel]:not([hidden])",
        ));
        if (!openPanel) return;
        const slug = openPanel.getAttribute("data-term-disclosure-panel");
        const trigger = /** @type {HTMLElement|null} */ (document.querySelector(
          "[data-term-disclosure-trigger='" + slug + "']",
        ));
        if (!trigger) return;
        openPanel.hidden = true;
        trigger.setAttribute("aria-expanded", "false");
        trigger.focus();
      });
    }
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
          // Issue #576: route through the shared clipboard
          // helper (window.__dixieCopyText) so the cheatsheet
          // per-row copy uses the same code path. Same
          // secure-context fallback as the legacy impl.
          const copyText = window.__dixieCopyText;
          if (!copyText) return;
          await copyText(path);
          showToast("Path copied.", "success");
        } catch (error) {
          showToast("Could not copy the path. Long-press to select.", "error");
        }
      });
    });
  }

  /**
   * @param {HTMLFormElement} form
   */
  /** @param {HTMLFormElement} form @returns {Record<string, string | null>} */
function currentBrowseStateFromForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return {};
    }
    const data = new FormData(form);
    /**
     * @param {string} key
     * @returns {string | null}
     */
    const get = (key) => {
      const value = data.get(key);
      return typeof value === "string" ? value : null;
    };
    return {
      page: get("page") || "1",
      page_size: get("page_size") || "100",
      scope: get("scope") || "all",
      sort: get("sort") || "display_id_asc",
      entry_type: get("entry_type") || "",
      unit: get("unit") || "",
      buried_in: get("buried_in") || "",
      pension_state: get("pension_state") || "",
      review_status: get("review_status") || "",
      confederate_home_status: get("confederate_home_status") || "",
    };
  }

  /**
   * @param {HTMLFormElement} form
   * @param {Record<string, unknown> | null | undefined} state
   */
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

  /**
   * @param {Record<string, string | null>} current
   * @param {Record<string, string | null>} saved
   */
  function browseStateDiffers(current, saved) {
    return ["page", "page_size", "scope", "sort", "entry_type", "unit", "buried_in", "pension_state", "review_status", "confederate_home_status"]
      .some((key) => String(current[key] || "") !== String(saved[key] || ""));
  }

  /**
   * @param {Document | HTMLElement} root
   */
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

  /**
   * @param {Document | HTMLElement} [root]
   */
  function updateBrowseSelectionStatus(root = document) {
    const selected = loadBrowseSelection();
    root.querySelectorAll("[data-browse-selection-status]").forEach((node) => {
      if (node instanceof HTMLElement) {
        node.textContent = selected.length > 0
          ? `${selected.length} record(s) selected across Browse pages and filters.`
          : "Select records across pages to keep a working set while you browse.";
      }
    });
    // Toggle bulk-tag toolbar visibility
    const toolbar = root.querySelector("#browse-bulk-toolbar");
    if (toolbar) {
      toolbar.classList.toggle("hidden", selected.length === 0);
      // Populate the hidden input with current selection
      const idsInput = toolbar.querySelector("[data-browse-bulk-ids]");
      if (idsInput instanceof HTMLInputElement) {
        idsInput.value = selected.join(",");
      }
    }
  }

  /**
   * @param {Document | HTMLElement} root
   */
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

  /**
   * @param {Document | HTMLElement} [root]
   */
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

  /** @param {Element} el @param {boolean} busy */
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

  /** @param {Element} el @param {boolean} busy */
function setBusyState(el, busy) {
    if (!(el instanceof HTMLElement)) {
      return;
    }
    if (busy) {
      el.setAttribute("aria-busy", "true");
      if (el instanceof HTMLButtonElement) {
        // data-busy-label: opt-in label swap while busy. Captures
        // the original textContent so the finally block can restore
        // it. The convention is opt-in per form (the initial-setup
        // form opts in; forms that don't care don't) so the global
        // helper stays a no-op for buttons without the attribute.
        const busyLabel = el.getAttribute("data-busy-label");
        if (busyLabel && !el.dataset.idleLabel) {
          el.dataset.idleLabel = el.textContent || "";
        }
        if (busyLabel) {
          el.textContent = busyLabel;
        }
        el.disabled = true;
      }
      return;
    }
    el.removeAttribute("aria-busy");
    if (el instanceof HTMLButtonElement) {
      // Restore the original label captured in the busy branch.
      if (el.dataset.idleLabel) {
        el.textContent = el.dataset.idleLabel;
        delete el.dataset.idleLabel;
      }
      el.disabled = false;
    }
  }

  // formIsNewSoldierWithEmptyNames detects the new-soldier form
  // (action targets /soldiers or /soldiers/new + carries the
  // ef-first_name and ef-last_name inputs) with both names empty
  // after trim. The check is intentionally narrow so the create
  // gate doesn't fire on edit forms (which carry the same input
  // ids via the entry_form.templ partial) or on synthetic forms.
  /** @param {HTMLFormElement} form */
function formIsNewSoldierWithEmptyNames(form) {
    if (!(form instanceof HTMLFormElement)) {
      return false;
    }
    const action = (form.getAttribute("action") || "").toLowerCase();
    const isCreateAction = action === "/soldiers" || action === "/soldiers/new" || action.endsWith("/soldiers/new");
    if (!isCreateAction) {
      return false;
    }
    // Read the method via getAttribute so a method="patch" form
    // doesn't get normalized to "get" by the HTMLFormElement
    // IDL getter (see #428 — same browser quirk as the main
    // dispatcher fix below).
    const methodAttr = (form.getAttribute && form.getAttribute("method")) || "";
    const method = String(methodAttr || "GET").toUpperCase();
    if (method !== "POST") {
      return false;
    }
    const first = form.querySelector("#ef-first_name");
    const last = form.querySelector("#ef-last_name");
    if (
      !(first instanceof HTMLInputElement)
      || !(last instanceof HTMLInputElement)
    ) {
      return false;
    }
    const firstEmpty = String(first.value || "").trim() === "";
    const lastEmpty = String(last.value || "").trim() === "";
    return firstEmpty && lastEmpty;
  }

  // dispatchUtilitySubmit is the canonical dispatcher for
  // utility-form submits — forms that do NOT carry data-dixie-submit
  // and whose submit handler runs a local side-effect instead of a
  // navigation/data action (e.g. saveCurrentQueueAsPresetPage,
  // a future "Save as draft" button). Wraps the form with a
  // submit listener that preventDefaults and runs the callback.
  //
  // Symmetric with dispatchDixieDataForm (which handles nav/data
  // submits on data-dixie-submit forms). Together they are the
  // two valid submit semantics; the htmx-guard probe recognizes both.
  //
  // Signature: dispatchUtilitySubmit(form, callback) — `form` is
  // an HTMLFormElement, `callback` is a function that takes the
  // form as its only argument. The callback's return value is
  // ignored; any thrown error propagates.
  //
  // The submit event listener is attached exactly once per
  // (form, callback) pair via a marker dataset attribute, so
  // callers can invoke this helper idempotently.
  /** @param {HTMLFormElement} form @param {(form: HTMLFormElement) => void} callback */
function dispatchUtilitySubmit(form, callback) {
    if (!(form instanceof HTMLFormElement)) return;
    if (form.dataset && form.dataset.utilitySubmitInstalled === "true") return;
    if (form.dataset) form.dataset.utilitySubmitInstalled = "true";
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      callback(form);
    });
  }

  // dispatchSubmitPrep is the canonical helper for submit-prep
  // listeners — body code runs a side-effect (stage hidden fields,
  // persist a preference, etc.) and then ALLOWS the submit to
  // continue. Use this when a <form data-dixie-submit="true"> (or
  // any other form that has its own submit semantics downstream)
  // needs a pre-submit hook.
  //
  // Signature: dispatchSubmitPrep(form, callback) — caller is
  // responsible for ensuring the listener body actually wants to
  // run; helpers below filter by form.matches("...") before
  // calling. Unlike dispatchUtilitySubmit, this does NOT
  // preventDefault — the natural submit flow continues.
  //
  // As with dispatchUtilitySubmit, the listener is installed exactly
  // once per form. Marker dataset attribute is shared with the
  // utility-submit install so the two helpers' install statuses
  // don't conflict (in practice a form uses one or the other,
  // not both).
  /** @param {HTMLFormElement} form @param {(form: HTMLFormElement) => void} callback */
function dispatchSubmitPrep(form, callback) {
    if (!(form instanceof HTMLFormElement)) return;
    if (form.dataset && form.dataset.utilitySubmitInstalled === "true") return;
    if (form.dataset) form.dataset.utilitySubmitInstalled = "true";
    form.addEventListener("submit", () => {
      callback(form);
    });
  }

  /** @param {EventTarget | HTMLFormElement} button */
async function dispatchDixieDataForm(button) {
    // Issue #248: when a button carries a data-action URL, that
    // URL represents the click target's intent and wins over the
    // parent form's action. The earlier code only honored
    // data-action in the bare-button (no parent form) branch; when
    // the button lived inside a data-dixie-submit form (e.g. the
    // per-row "Mark as Resolved" button on /review-queue, which is
    // wrapped by the bulk-action form), the form's action won and
    // the fetch hit /review-queue/bulk with no bulk_action field.
    // Build the form from the button's intent when data-action is
    // present, then fall back to the parent form. data-method
    // drives the method on the synthetic path; everything else
    // (data-confirm, data-results-target) is read from either the
    // button or the form via the existing dataset lookup.
    let form;
    if (button instanceof HTMLFormElement) {
      form = button;
    } else if (button instanceof HTMLElement) {
      const dataAction = (button.getAttribute && button.getAttribute("data-action")) || "";
      if (dataAction) {
        const method = button.getAttribute("data-method") === "DELETE" ? "DELETE" : "POST";
        const synthetic = document.createElement("form");
        synthetic.action = dataAction;
        synthetic.method = method;
        // Copy data-* attributes from the button so the synthetic
        // form can drive the same dispatch conventions (data-confirm,
        // data-results-target, data-reload-on-success) that the
        // surrounding page-level form would have provided. This is
        // how the per-row "Mark as Resolved" button (issue #248 +
        // #250) wires up its reload-on-success without being a child
        // of a real form.
        for (const attr of Array.from(button.attributes)) {
          if (attr.name.startsWith("data-") && attr.name !== "data-action" && attr.name !== "data-method") {
            synthetic.setAttribute(attr.name, attr.value);
          }
        }
        form = synthetic;
      } else {
        form = button.closest("form");
      }
    }
    if (!(form instanceof HTMLFormElement)) {
      return false;
    }
    const submitter = button instanceof HTMLElement ? button : null;
    const confirmMessage = (submitter && submitter.dataset && submitter.dataset.confirm)
      || (form.dataset && form.dataset.confirm);
    if (confirmMessage && !window.confirm(confirmMessage)) {
      return false;
    }
    // Respect the disabled state on the submitter. Native HTML
    // forms ignore clicks on disabled submit buttons, but the
    // JS dispatcher can still be invoked via dispatchEvent() or
    // by WebView2 quirks that don't fully honor the disabled
    // attribute. Per the HTML spec for FormData(form, submitter),
    // a disabled submitter produces an EMPTY entry list — so the
    // fetch would go out with no body and the server would 400
    // (e.g. Source Record ▲/▼ on the soldier card was hitting
    // "Position must be a positive integer." because the position
    // hidden input wasn't included in FormData). Bailing here is
    // a defense-in-depth net; the visible UI still relies on
    // `disabled` for the user-facing affordance.
    if (submitter instanceof HTMLButtonElement && submitter.disabled) {
      return false;
    }
    setBusyState(submitter || form, true);
    setBusyGroupState(submitter || form, true);
    try {
      const explicitMethod = (form.getAttribute && form.getAttribute("method")) || "";
      // Read the method from the HTML attribute (not form.method
      // IDL), because browsers normalize unsupported values
      // (PATCH, PUT, DELETE) to "get" via the HTMLFormElement
      // IDL getter per the HTML spec. Using form.method here
      // would silently turn every PATCH/PUT reorder form into a
      // GET against a route that only accepts PATCH — see
      // issue #428 for the smoke repro on Source Records and
      // event sources.
      /** @type {RequestInit} */
      const fetchOptions = { method: explicitMethod ? explicitMethod.toUpperCase() : "POST" };
      // Only attach a body for non-GET / non-HEAD requests. Bare-button
      // synthetic forms have no FormData to attach anyway.
      const methodUpper = String(fetchOptions.method).toUpperCase();
      if (methodUpper !== "GET" && methodUpper !== "HEAD") {
        // Issue #247: when the form has multiple submit buttons
        // sharing the same name (e.g. review-queue's Ignore +
        // Delete with name="bulk_action"), `new FormData(form)`
        // silently drops the submitter's name+value because the
        // synthetic fetch is not a real form submission. Pass the
        // submitter as the second arg AND, as a belt-and-suspenders
        // fallback for browsers that don't honor the submitter
        // argument in synthetic FormData construction, manually
        // append the submitter's entry if FormData omitted it.
        // Issue #248: only pass the submitter when it is a real
        // submit button of the form. A type="button" trigger
        // (e.g. the per-row "Mark as Resolved" button on
        // /review-queue) is not a form submitter; passing it to
        // FormData throws "not a submit button" in Chromium. For
        // type="button" triggers, the form's named controls are
        // enough (the data-action URL is what matters; the body
        // carries the form's data, not the trigger's name/value).
        const isSubmitButton = button instanceof HTMLButtonElement
          && button.type === "submit"
          && button.form === form;
        if (button instanceof HTMLElement && button.closest("form")) {
          const fd = isSubmitButton ? new FormData(form, button) : new FormData(form);
          if (isSubmitButton && button instanceof HTMLButtonElement && button.name && fd.get(button.name) === null) {
            fd.append(button.name, button.value);
          }
          fetchOptions.body = fd;
        } else {
          fetchOptions.body = new FormData();
        }
      }
      // review queue with NeedsReview=true + ReviewReason set.
      // Must run AFTER fetchOptions.body is constructed so we can
      // append confirm_empty_name to it. The original code ordered
      // the empty-name confirm before the fetchOptions declaration,
      // leaving fetchOptions in the temporal dead zone on the empty-
      // name path — TypeScript flagged the read-before-declare here
      // as TS2304 (slice-2 fix in the typecheck-baseline work).
      if (formIsNewSoldierWithEmptyNames(form)) {
        if (!window.confirm("Saving a record with no name. It will be marked for review. Continue?")) {
          return false;
        }
        if (fetchOptions.body instanceof FormData) {
          fetchOptions.body.append("confirm_empty_name", "1");
        }
      }
      // Read the URL from the form's `action` content attribute
      // (not the `form.action` IDL getter). Chromium returns a
      // RadioNodeList instead of the action-attribute string when
      // the form contains any descendant element named "action"
      // (e.g. the feedback modal's Save + Send submit buttons,
      // both `name="action"`). `form.getAttribute('action')` reads
      // the raw attribute and is unaffected by descendant named
      // controls. Tracked in #571.
      const requestUrl = form.getAttribute('action') || window.location.pathname;
      // [Wails-PATCH] Wails v2.12.0 strips the body from
      // PATCH/PUT/DELETE requests sent through the
      // wails.localhost custom protocol — confirmed by the
      // dispatcher's body probe showing FormData(entries=[...])
      // at fetch time but the server receiving an empty body.
      // The server's requestMethodOverride middleware
      // (internal/appshell/app.go:487) already accepts the
      // standard X-HTTP-Method-Override header pattern, so
      // we rewrite non-GET/POST methods to POST + header when
      // we're inside the Wails runtime. Plain Chromium (the
      // audit harness) keeps the real PATCH so Playwright
      // tests still observe the genuine method.
      if (
        fetchOptions
        && fetchOptions.method
        && fetchOptions.method !== "GET"
        && fetchOptions.method !== "HEAD"
        && fetchOptions.method !== "POST"
        && typeof requestUrl === "string"
        && (requestUrl.indexOf("wails.localhost") >= 0
          || requestUrl.indexOf("://wails.") >= 0)
      ) {
        fetchOptions.headers = Object.assign({}, fetchOptions.headers, {
          "X-HTTP-Method-Override": String(fetchOptions.method).toUpperCase(),
        });
        fetchOptions.method = "POST";
      }
      // [Wails-FormData] Wails v2.12.0's custom-protocol
      // HTTP server appears to strip multipart FormData
      // bodies (the FormData entries show up at fetch time
      // but the Go handler sees an empty form). JSON bodies
      // (used by /debug/client-logs) DO survive — and so do
      // urlencoded bodies per spec. Convert FormData to
      // urlencoded when running inside the Wails runtime so
      // the asset server forwards the form fields verbatim.
      // The audit harness (vanilla Chromium against
      // dixiedata-web) keeps using FormData so large bodies
      // and file uploads keep working there.
      if (
        fetchOptions
        && fetchOptions.body
        && typeof FormData !== "undefined"
        && fetchOptions.body instanceof FormData
        && typeof URLSearchParams !== "undefined"
        && typeof requestUrl === "string"
        && requestUrl.indexOf("wails.localhost") >= 0
      ) {
        try {
          const params = new URLSearchParams();
          for (const [k, v] of fetchOptions.body.entries()) {
            params.append(k, typeof v === "string" ? v : (v && v.name) || "");
          }
          fetchOptions.body = params.toString();
          fetchOptions.headers = Object.assign({}, fetchOptions.headers, {
            "Content-Type": "application/x-www-form-urlencoded",
          });
        } catch (_) {
          // leave FormData intact on any encoding error
        }
      }
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
      const draftClearForm = button instanceof HTMLElement ? button.closest("form") : null;
      if (draftClearForm instanceof HTMLFormElement && response.ok) {
        clearDraftForForm(draftClearForm);
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
      // Issue #250: data-reload-on-success is a one-attribute opt-in
      // for "inline action that mutates the page state, no fragment
      // available — just reload the page so the user sees the new
      // state + the badge re-fetches + the toast shows immediately
      // (before the next page load via savePendingToast)." Used by
      // the per-row "Mark as Resolved" button on /review-queue so
      // a single click removes the item + updates the top-nav badge
      // + shows the confirmation toast without a full nav. We
      // surface the toast now (not via savePendingToast) because
      // the reload fires immediately and the user shouldn't have to
      // wait for the next page load to see confirmation.
      const reloadOnSuccess = (form.dataset && form.dataset.reloadOnSuccess) === "true";
      if (reloadOnSuccess && responseOk && !redirectTo) {
        if (toastMessage) {
          showToast(toastMessage, toastKind);
        }
        window.location.reload();
        return true;
      }
      // Issue #244: data-clear-share-queue-on-success is a
      // one-attribute opt-in for forms that commit a .ddshare
      // export. On success the share queue is cleared (the
      // staged items have been sent), the pill hides, the
      // /share/queue page re-renders to the empty state, and
      // the toast surfaces immediately (not via savePendingToast
      // because the existing path is sufficient and we do not
      // want a delayed toast on the success-only path). Used
      // by the Bulk Export form on /share/queue and any other
      // /export/shared-archive?subset=1 submitter.
      const clearShareQueueOnSuccess = (form.dataset && form.dataset.clearShareQueueOnSuccess) === "true";
      if (clearShareQueueOnSuccess && responseOk && !redirectTo) {
        if (typeof writeShareQueue === "function") {
          writeShareQueue([]);
        }
        if (typeof renderShareQueuePage === "function") {
          renderShareQueuePage();
        }
        if (toastMessage) {
          showToast(toastMessage, toastKind);
        }
        return true;
      }
      if (resultsTargetSelector && !redirectTo && responseOk) {
        // Issue #402 follow-up: uiid ids contain dots (e.g.
        // "panel.event.detail.images"). querySelector("#panel.event.detail.images")
        // parses the dots as class selectors and returns null. When
        // the selector is an id reference, fall back to the
        // attribute-selector form so the swap targets the right node.
        const target = document.querySelector(
          resultsTargetSelector.startsWith("#") && resultsTargetSelector.includes(".")
            ? `[id="${resultsTargetSelector.slice(1)}"]`
            : resultsTargetSelector,
        );
        if (target instanceof HTMLElement) {
          const html = await response.text();
          target.innerHTML = html;
          initializeDynamicContent();
        }
      }
      const requestState = {
        scrollX: window.scrollX,
        scrollY: window.scrollY,
      };
      rememberRedirectState(submitter || form, redirectTo || window.location.pathname, "", requestState);
      // Source Record reorder highlight (UX pass, see AGENTS.md
      // "Wails runtime hazards" + soldier_card.templ). When the
      // request was a successful Source reorder — PATCH/POST to
      // /soldiers/{id}/sources/{sourceId}/position with a 2xx
      // response — stash the moved source record's ID in
      // sessionStorage so the next page load can flash the
      // row that just moved. The sessionStorage is cleared
      // inside the DOMContentLoaded handler after the
      // animation finishes, so a single highlight fires per
      // successful reorder.
      if (
        responseOk
        && typeof form === "object"
        && form
        && form.action
        && typeof URL !== "undefined"
      ) {
        try {
          const u = new URL(form.action, window.location.href);
          const m = u.pathname.match(/^\/soldiers\/(\d+)\/sources\/(\d+)\/position$/);
          if (m && response.status >= 200 && response.status < 300) {
            try {
              window.sessionStorage.setItem("dixiedata.lastMovedSource", m[2]);
            } catch (_) {
              // sessionStorage may be disabled (private mode);
              // the highlight is a nice-to-have, not essential.
            }
          }
        } catch (_) {
          // non-URL form.action (e.g. relative); skip
        }
      }
      if (redirectTo) {
        // Issue #534: when the user preference is "toast-only",
        // suppress the post-export navigation to /jobs/{id} but
        // still fire the toast (the toast header was already
        // captured at the top of this dispatcher and gets shown
        // when the form is not the close-feedback-modal path).
        // The preference is read from the per-page
        // <html data-export-surface="..."> attribute the
        // /settings handler sets on every page render, so the
        // dispatcher consults the same source the UI does.
        const exportSurface = (document.documentElement && document.documentElement.dataset
          ? document.documentElement.dataset.exportSurface
          : "") || "jobs-page";
        const suppressNav = exportSurface === "toast-only";
        if (!suppressNav) {
          window.location.assign(redirectTo);
        }
      }
      return true;
    } finally {
      setBusyState(submitter || form, false);
      setBusyGroupState(submitter || form, false);
    }
  }

  /** @param {string} href */
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

  /** @type {HTMLElement | null} */
  let overlayModalRestoreFocus = null;

  /** @param {Element} modal */
  function showOverlayModal(modal) {
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    overlayModalRestoreFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    const focusable = modal.querySelectorAll(OVERLAY_MODAL_FOCUSABLE);
    if (focusable.length > 0) {
      if (focusable[0] instanceof HTMLElement) {
        focusable[0].focus();
      }
    } else {
      modal.setAttribute("tabindex", "-1");
      modal.focus();
    }
    document.addEventListener("keydown", overlayModalKeydown, true);
  }

  /** @param {Element} modal */
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

  /** @param {KeyboardEvent} event */
function overlayModalKeydown(event) {
    if (event.key !== "Tab") {
      return;
    }
    const openModal = document.querySelector('[role="dialog"][aria-modal="true"]:not(.hidden)');
    if (!(openModal instanceof HTMLElement)) {
      return;
    }
    const focusable = Array.from(openModal.querySelectorAll(OVERLAY_MODAL_FOCUSABLE)).filter(
      (el) => el instanceof HTMLElement && !el.hasAttribute("disabled") && el.offsetParent !== null,
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
      if (last instanceof HTMLElement) {
        last.focus();
      }
      return;
    }
    if (!event.shiftKey && active === last) {
      event.preventDefault();
      if (first instanceof HTMLElement) {
        first.focus();
      }
      return;
    }
  }

  // Issue #234: cache the lazy-loaded print-records fragment so
  // subsequent modal opens in the same session are free. Cleared
  // by invalidatePrintRecordsCache() when an archive-mutating
  // action completes (export template save, share queue edit, etc.)
  // so the next open re-fetches.
  /** @type {string | null} */
  let printRecordsFragmentCache = null;
  /** @type {Promise<string | null> | null} */
  let printRecordsFragmentInflight = null;

  function invalidatePrintRecordsCache() {
    printRecordsFragmentCache = null;
    printRecordsFragmentInflight = null;
  }

  // loadPrintRecordsFragment fetches the fragment on first open and
  // swaps into [data-print-config-body]. On subsequent opens with a
  // valid cache the body is restored from memory and no network
  // request fires. The placeholder rendered by the templ (empty
  // chrome, "Loading…" fallback) is overwritten either way.
  /** @param {Element} modal */
function loadPrintRecordsFragment(modal) {
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const body = modal.querySelector("[data-print-config-body]");
    if (!(body instanceof HTMLElement)) {
      return;
    }
    if (printRecordsFragmentCache !== null) {
      body.innerHTML = printRecordsFragmentCache;
      onPrintRecordsFragmentReady(modal);
      return;
    }
    if (printRecordsFragmentInflight !== null) {
      // Reuse the in-flight promise to dedupe concurrent opens.
      printRecordsFragmentInflight
        .then((html) => {
          if (html === null) {
            return null;
          }
          if (document.body.contains(modal)) {
            body.innerHTML = html;
            onPrintRecordsFragmentReady(modal);
          }
          return html;
        })
        .catch((err) => {
          // The inflight fragment fetch failed (network error,
          // server 500, etc.). The catch-all is intentional — the
          // user is already on the modal-open code path that has
          // its own status/empty-state plumbing; this promise is
          // only the dedup-cache. Log so the operator can see it
          // but don't double-surface to the user.
          // See error-handling.md "JS catches (client-side)".
          console.warn("print records fragment dedup failed", err);
        });
      return;
    }
    printRecordsFragmentInflight = fetch("/share/print-records-fragment", {
      headers: { Accept: "text/html" },
    })
      .then((resp) => {
        if (!resp.ok) {
          throw new Error("fragment fetch failed: " + resp.status);
        }
        return resp.text();
      })
      .then((html) => {
        printRecordsFragmentCache = html;
        printRecordsFragmentInflight = null;
        if (document.body.contains(modal)) {
          body.innerHTML = html;
          onPrintRecordsFragmentReady(modal);
        }
        return html;
      })
      .catch((err) => {
        printRecordsFragmentInflight = null;
        // Issue #384 / Slice 1: the user clicked Print, the modal opened,
        // and then nothing happened. console.warn alone is invisible to
        // most users. Swap the modal body for an EmptyStateError-style
        // inline message AND fire a toast so the failure is unmistakable.
        if (typeof console !== "undefined") {
          console.warn("print-records fragment load failed", err);
        }
        if (document.body.contains(modal)) {
          body.innerHTML = `
            <div class="empty-state empty-state-error" role="alert" data-empty-state-kind="error">
              <p class="text-sm font-semibold text-red-800">
                <span aria-hidden="true" class="mr-1">⚠</span>Could not load print options.
              </p>
              <p class="mt-1 text-sm text-red-700">
                Check your connection and try opening Print again.
              </p>
            </div>`;
        }
        if (typeof showToast === "function") {
          showToast("Could not load print options.", "error");
        }
        return null;
      });
  }

  // onPrintRecordsFragmentReady wires the just-swapped body:
  // re-enables the submit button, runs the existing print-config
  // filter wiring, and refreshes the preview. Mirrors what
  // openPrintConfigModal does for the modal itself, scoped to the
  // fragment that just arrived.
  /** @param {Element} modal */
function onPrintRecordsFragmentReady(modal) {
    const submit = modal.querySelector("[data-print-config-submit]");
    if (submit instanceof HTMLButtonElement) {
      submit.disabled = false;
    }
    applyPrintRecordFilter();
    applyPrintBuriedFilter();
    refreshPrintConfigPreview();
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
    installPrintConfigPreview();
    refreshPrintConfigPreview();
    installExportTemplates();
    refreshExportTemplates();
    showOverlayModal(modal);
    // Issue #234: lazy-load the print-records fragment into the
    // [data-print-config-body] placeholder. The fragment endpoint
    // (/share/print-records-fragment) returns the filter panel +
    // record picker as a swap target. Cached on first load so
    // subsequent modal opens in the same session are free.
    loadPrintRecordsFragment(modal);
    // installShareQueueGlobals + updateShareQueuePill run at
    // boot (see DOMContentLoaded) so every + Queue button works
    // on first paint without first opening this modal. The calls
    // here are kept as a safety net for the (currently unused)
    // htmx-swap-without-pageload flow; both functions are
    // idempotent.
  }

  function closePrintConfigModal() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    hideOverlayModal(modal);
  }

  // initializePersonRecordPicker wires every
  // [data-person-record-picker-clear] button to the same
  // behavior the inline-onclick at the templ site used to
  // do: clear the closest [data-person-record-picker-target]
  // ancestor's innerHTML so the picker shell disappears
  // from the surrounding article/event page. Replaces
  // `onclick="document.querySelector(...).innerHTML = ''"`
  // with the shared data-* + JS-initializer pattern
  // (issue #574). Idempotent via the per-element flag
  // matching the initializeCopyPathButtons shape.
  function initializePersonRecordPicker() {
    document.querySelectorAll("[data-person-record-picker-clear]").forEach((button) => {
      if (button.__pickerClearBound) {
        return;
      }
      button.__pickerClearBound = true;
      button.addEventListener("click", () => {
        const picker = button.closest("[data-person-record-picker]");
        const searchRoot = picker instanceof HTMLElement ? picker.parentElement : null;
        // The picker opens into the page-level
        // [data-person-record-picker-target] ancestor. Walk
        // up from the picker's parent looking for it (the
        // picker shell is inside the target div, which is
        // inside the consumer form, which is inside the
        // page). closest() finds the first one above so a
        // nested consumer (e.g. a future modal wrapper)
        // still targets the nearest target.
        let cursor = searchRoot;
        while (cursor && !(cursor instanceof Element && cursor.matches("[data-person-record-picker-target]"))) {
          cursor = cursor instanceof Element ? cursor.parentElement : null;
        }
        if (cursor instanceof HTMLElement) {
          cursor.innerHTML = "";
        }
      });
    });
  }

  // initializeMarkdownCheatsheet wires the per-row Copy
  // buttons on the Article editor's Markdown syntax
  // cheatsheet (issue #565 / #576). The data attrs are
  // already shipped by the templ component
  // (`data-md-cheatsheet-copy-key` for the row id,
  // `data-md-cheatsheet-copy-value` for the example source);
  // the JS hookup was missing, so the buttons did nothing.
  // The wire-up goes through the shared clipboard helper
  // (window.__dixieCopyText) so the cheatsheet + the
  // [data-copy-path] copy-path buttons share one code
  // path (DRY §1.1). Idempotent via the per-element guard
  // matching the rest of the initializer family.
  function initializeMarkdownCheatsheet() {
    document.querySelectorAll("[data-md-cheatsheet-copy-key]").forEach((button) => {
      if (button.__cheatsheetCopyBound) {
        return;
      }
      button.__cheatsheetCopyBound = true;
      button.addEventListener("click", async () => {
        const value = button.getAttribute("data-md-cheatsheet-copy-value") || "";
        if (!value) {
          showToast("Nothing to copy.", "error");
          return;
        }
        const copyText = window.__dixieCopyText;
        if (!copyText) {
          showToast("Clipboard helper unavailable.", "error");
          return;
        }
        try {
          await copyText(value);
          // Truncate the toast text for long examples so the
          // toast card doesn't grow a long paragraph per click.
          const preview = value.length > 40 ? value.slice(0, 40) + "…" : value;
          showToast("Copied: " + preview, "success");
        } catch (error) {
          showToast("Could not copy. Long-press to select.", "error");
        }
      });
    });
  }

  // ----- Dismiss button on /jobs/{id} (issue #249) -----
  // The templ renders a [data-dismiss-job] button with a
  // data-dismiss-target carrying the kind-specific fallback
  // (Job.DismissTargetPath()). On click, prefer the same-origin
  // document.referrer if it isn't a /jobs/* page itself (avoids
  // loops when the user dismisses a chain of jobs). Otherwise
  // use the templ fallback. Both /jobs/1 → /jobs/2 (cross-job
  // navigation) and absent/off-origin referrer are handled.
  function installDismissJobButtons() {
    document.addEventListener("click", (ev) => {
      const target = ev.target;
      if (!(target instanceof HTMLElement)) return;
      const btn = target.closest("[data-dismiss-job]");
      if (!(btn instanceof HTMLElement)) return;
      ev.preventDefault();
      const fallback = btn.getAttribute("data-dismiss-target") || "/share";
      const destination = pickDismissTarget(fallback);
      window.location.assign(destination);
    });
  }
  /** @param {string} fallback @returns {string} */
function pickDismissTarget(fallback) {
    const ref = String(document.referrer || "");
    if (!ref) return fallback;
    let url;
    try {
      url = new URL(ref);
    } catch (_) {
      return fallback;
    }
    if (url.origin !== window.location.origin) return fallback;
    const path = url.pathname || "";
    // Avoid loops: the referer must not be a /jobs/* page or a
    // /jobs/{id}/report sub-page (which is a sub-page of the
    // status page).
    if (path === "/jobs" || path.startsWith("/jobs/")) return fallback;
    return path + (url.search || "");
  }

  // ----- Share Queue modal (issue #182) -----
  // The queue lives in localStorage under
  // dixiedata.share-queue. The server-rendered modal body is
  // the shell; JS hydrates the staged-records list from
  // localStorage on open, keeps the persistent pill in sync,
  // and submits the modal's selected_ids to
  // /export/shared-archive?subset=1.
  const SHARE_QUEUE_STORAGE_KEY = "dixiedata.share-queue";
  /** @returns {number[]} */
function readShareQueue() {
    try {
      const raw = window.localStorage && window.localStorage.getItem(SHARE_QUEUE_STORAGE_KEY);
      if (!raw) return [];
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return [];
      return parsed.filter((n) => typeof n === "number" && Number.isFinite(n) && n > 0);
    } catch (err) {
      return [];
    }
  }
  /** @param {number[]} ids */
function writeShareQueue(ids) {
    try {
      if (!window.localStorage) return;
      window.localStorage.setItem(SHARE_QUEUE_STORAGE_KEY, JSON.stringify(ids));
    } catch (err) {
      // Localstorage may be disabled (private mode); ignore so
      // the UX degrades silently rather than throwing.
    }
    updateShareQueuePill(ids);
  }
  /**
   * @param {number[] | null} [ids] omitted → derive from readShareQueue()
   */
function updateShareQueuePill(ids) {
    const pill = document.querySelector("[data-share-queue-pill]");
    if (!(pill instanceof HTMLElement)) return;
    if (!ids || ids.length === 0) {
      pill.classList.add("hidden");
      return;
    }
    pill.classList.remove("hidden");
    const counter = pill.querySelector("[data-share-queue-pill-count]");
    if (counter instanceof HTMLElement) {
      counter.textContent = String(ids.length);
    }
  }
  /** @param {number} id */
function addToShareQueue(id) {
    if (typeof id !== "number" || id <= 0) return;
    const ids = readShareQueue();
    if (ids.indexOf(id) !== -1) return;
    ids.push(id);
    writeShareQueue(ids);
    updateShareQueuePill(ids);
  }
  /** @param {number} id */
function removeFromShareQueue(id) {
    const ids = readShareQueue().filter((n) => n !== id);
    writeShareQueue(ids);
    updateShareQueuePill(ids);
  }
  function installShareQueueGlobals() {
    // Per-row [+] Queue buttons. The persistent pill at the
    // bottom of every page and the Build Share Archive button on
    // /share/exports are now <a href="/share/queue"> links that
    // navigate to the /share/queue management page directly
    // (issue #310, PR 1: retarget triggers to navigate). The
    // pill still toggles its visibility + count via
    // updateShareQueuePill() using the data-share-queue-pill
    // attribute and the data-share-queue-pill-count child.
    document.addEventListener("click", (event) => {
      const target = event.target;
      if (!(target instanceof HTMLElement)) return;
      const addBtn = target.closest("[data-share-queue-add]");
      if (addBtn instanceof HTMLElement) {
        const idAttr = addBtn.getAttribute("data-share-queue-add");
        const id = idAttr ? parseInt(idAttr, 10) : 0;
        if (id > 0) {
          addToShareQueue(id);
          // Visual feedback: brief "Added" pulse.
          const original = addBtn.textContent;
          addBtn.textContent = "Added";
          setTimeout(() => {
            if (addBtn.textContent === "Added") addBtn.textContent = original;
          }, 800);
        }
        event.preventDefault();
        return;
      }
    });
    installShareQueuePage();
    installShareQueuePresetsPage();
  }

  // ----- /share/queue management page (issue #193) -----
  // The page hydrates from localStorage on load. We render
  // the table client-side (since the localStorage queue is
  // client-side truth) AND keep the server-rendered stub in
  // sync: any Remove or Bulk-Remove clicks update the
  // localStorage queue, re-render the table, and update the
  // pill. The bulk-export form submits via dispatchDixieDataForm
  // to /export/shared-archive?subset=1 with the selected rows
  // injected as hidden selected_ids fields.
  /** @returns {number[]} */
function getSelectedIdsOnPage() {
    const inputs = document.querySelectorAll("input[type=checkbox][data-share-queue-page-select]:checked");
    /** @type {number[]} */
    const ids = [];
    inputs.forEach((el) => {
      if (!(el instanceof HTMLInputElement)) {
        return;
      }
      const v = el.value ? parseInt(el.value, 10) : 0;
      if (v > 0) ids.push(v);
    });
    return ids;
  }
  /** @param {string} text */
function pageSetStatus(text) {
    const slot = document.querySelector("[data-share-queue-page-status]");
    if (!(slot instanceof HTMLElement)) return;
    slot.textContent = text || "";
  }
  function syncShareQueuePageButtons() {
    const ids = getSelectedIdsOnPage();
    const removeBtn = document.querySelector("[data-share-queue-page-bulk-remove]");
    const exportBtn = document.querySelector("[data-share-queue-page-bulk-export]");
    const enabled = ids.length > 0;
    if (removeBtn instanceof HTMLButtonElement) removeBtn.disabled = !enabled;
    if (exportBtn instanceof HTMLButtonElement) exportBtn.disabled = !enabled;
  }
  /** @param {{ id: number, display_id: string, heading?: string, unit?: string, records?: string[], images?: string[] }} row @param {number} index @returns {HTMLTableRowElement} */
function shareQueuePageRowTemplate(row, index) {
    const tr = document.createElement("tr");
    tr.setAttribute("data-share-queue-page-row-id", String(row.id));
    tr.className = "border-b border-[rgba(141,116,64,0.18)]";
    tr.innerHTML = `
      <td class="px-2 py-3 align-top">
        <input type="checkbox" value="${row.id}" data-share-queue-page-select data-share-queue-page-label="${row.display_id}" class="h-4 w-4 rounded border-slate-300 text-[#8d7440] focus:ring-[#8d7440]" aria-label="Select ${row.display_id}"/>
      </td>
      <td class="px-2 py-3 align-top font-mono text-xs text-slate-500" data-share-queue-page-order>${index + 1}</td>
      <td class="px-2 py-3 align-top font-mono text-[#6f2c26]"><a href="/soldiers/${row.id}" class="hover:underline">${row.display_id}</a></td>
      <td class="px-2 py-3 align-top font-semibold text-[#22303d]">${row.heading || ""}</td>
      <td class="px-2 py-3 align-top text-slate-600">${row.unit || ""}</td>
      <td class="px-2 py-3 align-top text-slate-600">${row.records || 0}</td>
      <td class="px-2 py-3 align-top text-slate-600">${row.images || 0}</td>
      <td class="px-2 py-3 align-top"><button type="button" data-share-queue-page-remove-id="${row.id}" class="rounded border border-[rgba(111,44,38,0.35)] bg-white/85 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.12em] text-[#6f2c26] hover:bg-white">Remove</button></td>`;
    return tr;
  }
  async function renderShareQueuePage() {
    // Issue pending: the previous version targeted the tbody
    // (data-share-queue-page-body) and early-returned when
    // missing. That was wrong: when localStorage has items
    // but the user navigates here for the first time, the
    // server renders the empty-state branch (no tbody) and
    // the function silently no-ops, leaving the empty card
    // on screen even though the pill says "Share queue: 3".
    // Fix: target the section (panel.share-queue.list)
    // which is always present, fetch the page with the
    // current ids, and replace the section's inner contents
    // with the new section's inner contents. This handles
    // both empty→populated and populated→empty transitions
    // in one code path.
    const section = document.querySelector(`section[id="${ShareQueueListSectionID}"]`);
    if (!(section instanceof HTMLElement)) return;
    const ids = readShareQueue();
    const target = ids.length === 0 ? "/share/queue" : `/share/queue?ids=${ids.join(",")}`;
    let response;
    try {
      response = await fetch(target, { method: "GET" });
    } catch (err) {
      updateShareQueuePill(ids);
      syncShareQueuePageButtons();
      return;
    }
    if (!response.ok) {
      // Server failed: render a minimal placeholder so the
      // user sees the ids even if metadata lookup is broken.
      if (ids.length > 0) {
        const tbody = document.createElement("tbody");
        tbody.setAttribute("data-share-queue-page-body", "");
        ids.forEach((id, i) => tbody.appendChild(shareQueuePageRowTemplate({ id, display_id: "#" + id }, i)));
        section.replaceChildren(tbody);
      }
      updateShareQueuePill(ids);
      syncShareQueuePageButtons();
      return;
    }
    const html = await response.text();
    const doc = new DOMParser().parseFromString(html, "text/html");
    const freshSection = doc.querySelector(`section[id="${ShareQueueListSectionID}"]`);
    if (freshSection instanceof HTMLElement) {
      section.replaceChildren(...Array.from(freshSection.childNodes).map((n) => n.cloneNode(true)));
    }
    updateShareQueuePill(ids);
    syncShareQueuePageButtons();
  }
  // Issue pending: the section (id="panel.share-queue.list")
  // is the persistent install anchor; the tbody is transient
  // and depends on whether the page rendered the empty-state
  // or the rows. Always derive the install-target from the
  // section, never the tbody, so event handlers + flag persist
  // across re-renders.
  const ShareQueueListSectionID = "panel.share-queue.list";
  const ShareQueuePresetsSectionID = "panel.share-queue.presets";

  // shareQueuePresetStatusPage writes a transient message into the
  // presets panel on the /share/queue page. Ported from the modal
  // helper of the same name (share_queue_modal.templ) when the
  // modal was deleted in issue #310 PR 2; PR 3 re-mounts the
  // presets UI on the page.
  /** @param {Element} panel @param {string} text */
function shareQueuePresetStatusPage(panel, text) {
    if (!(panel instanceof HTMLElement)) return;
    const slot = panel.querySelector("[data-share-queue-preset-status]");
    if (!(slot instanceof HTMLElement)) return;
    if (!text) {
      slot.textContent = "";
      slot.classList.add("hidden");
      return;
    }
    slot.textContent = text;
    slot.classList.remove("hidden");
  }

  // saveCurrentQueueAsPresetPage POSTs the current localStorage
  // queue to /share/queue/presets and refreshes the preset list.
  // Mirrors the modal save handler; uses the page's presets panel
  // instead of querying the modal's [data-share-queue-modal].
  /** @param {Element} panel @param {HTMLFormElement} form */
async function saveCurrentQueueAsPresetPage(panel, form) {
    if (!(panel instanceof HTMLElement)) return;
    if (!(form instanceof HTMLFormElement)) return;
    const ids = readShareQueue();
    if (ids.length === 0) {
      shareQueuePresetStatusPage(panel, "Stage at least one Person Record before saving.");
      return;
    }
    const nameInput = form.querySelector("input[name=name]");
    const name = nameInput instanceof HTMLInputElement ? nameInput.value.trim() : "";
    if (!name) {
      shareQueuePresetStatusPage(panel, "Give the preset a name first.");
      return;
    }
    try {
      const fd = new FormData();
      fd.set("name", name);
      for (const id of ids) fd.append("soldier_ids", String(id));
      const response = await fetch("/share/queue/presets", { method: "POST", body: fd });
      if (response.status === 409) {
        shareQueuePresetStatusPage(panel, "A preset with that name already exists. Pick a different name.");
        return;
      }
      if (!response.ok) {
        shareQueuePresetStatusPage(panel, "Could not save the preset.");
        return;
      }
      if (nameInput instanceof HTMLInputElement) nameInput.value = "";
      shareQueuePresetStatusPage(panel, "Saved.");
      await refreshShareQueuePresetsPage(panel);
    } catch (err) {
      shareQueuePresetStatusPage(panel, "Could not save the preset.");
    }
  }

  // loadShareQueuePresetPage GETs /share/queue/presets/{id}/apply,
  // writes the returned soldier_ids back to localStorage, and
  // re-renders the page's table + pill. If the queue already has
  // items, confirms with the user (same as the deleted modal).
  /** @param {Element} panel @param {number} id */
async function loadShareQueuePresetPage(panel, id) {
    if (!(panel instanceof HTMLElement)) return;
    const ids = readShareQueue();
    if (ids.length > 0) {
      if (!window.confirm("Replace the current Share Queue with the preset's contents?")) {
        return;
      }
    }
    try {
      const response = await fetch(`/share/queue/presets/${id}/apply`, { method: "GET" });
      if (response.status === 404) {
        shareQueuePresetStatusPage(panel, "That preset no longer exists.");
        await refreshShareQueuePresetsPage(panel);
        return;
      }
      if (!response.ok) {
        shareQueuePresetStatusPage(panel, "Could not load the preset.");
        return;
      }
      const data = await response.json();
      const newIds = Array.isArray(data && data.soldier_ids) ? data.soldier_ids : [];
      writeShareQueue(newIds);
      renderShareQueuePage();
      shareQueuePresetStatusPage(panel, "Loaded preset.");
    } catch (err) {
      shareQueuePresetStatusPage(panel, "Could not load the preset.");
    }
  }

  // deleteShareQueuePresetPage DELETEs /share/queue/presets/{id}
  // and refreshes the list. Confirms with the user (same as the
  // deleted modal).
  /** @param {Element} panel @param {number} id */
async function deleteShareQueuePresetPage(panel, id) {
    if (!(panel instanceof HTMLElement)) return;
    if (!window.confirm("Delete this saved preset? This cannot be undone.")) {
      return;
    }
    try {
      const response = await fetch(`/share/queue/presets/${id}`, { method: "DELETE" });
      if (!response.ok) {
        shareQueuePresetStatusPage(panel, "Could not delete the preset.");
        return;
      }
      shareQueuePresetStatusPage(panel, "Deleted.");
      await refreshShareQueuePresetsPage(panel);
    } catch (err) {
      shareQueuePresetStatusPage(panel, "Could not delete the preset.");
    }
  }

  // refreshShareQueuePresetsPage fetches /share/queue/presets and
  // renders the preset list into the page's [data-share-queue-preset-list]
  // <ul>. The empty-state div is toggled based on whether any presets
  // returned.
  /** @param {Element} panel */
async function refreshShareQueuePresetsPage(panel) {
    if (!(panel instanceof HTMLElement)) return;
    const list = panel.querySelector("[data-share-queue-preset-list]");
    const empty = panel.querySelector("[data-share-queue-preset-empty]");
    if (!(list instanceof HTMLElement)) return;
    while (list.firstChild) list.removeChild(list.firstChild);
    let presets = [];
    try {
      const response = await fetch("/share/queue/presets", { method: "GET" });
      if (response.ok) {
        const data = await response.json();
        if (data && Array.isArray(data.presets)) presets = data.presets;
      }
    } catch (err) {
      // Network blip -- show the empty state rather than throwing.
    }
    for (const preset of presets) {
      const li = document.createElement("li");
      li.className = "flex items-center justify-between gap-2 rounded border border-[rgba(141,116,64,0.2)] bg-white/80 px-2 py-1";
      const label = document.createElement("span");
      label.className = "truncate font-semibold text-[#22303d]";
      label.textContent = preset.name;
      const right = document.createElement("div");
      right.className = "flex shrink-0 items-center gap-1";
      const load = document.createElement("button");
      load.type = "button";
      load.className = "rounded border border-[rgba(141,116,64,0.35)] bg-white/85 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.12em] text-[#8d7440] hover:bg-white";
      load.textContent = "Load";
      load.addEventListener("click", () => loadShareQueuePresetPage(panel, preset.id));
      const del = document.createElement("button");
      del.type = "button";
      del.className = "rounded border border-[rgba(111,44,38,0.35)] bg-white/85 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.12em] text-[#6f2c26] hover:bg-white";
      del.textContent = "Delete";
      del.addEventListener("click", () => deleteShareQueuePresetPage(panel, preset.id));
      right.appendChild(load);
      right.appendChild(del);
      li.appendChild(label);
      li.appendChild(right);
      list.appendChild(li);
    }
    if (empty instanceof HTMLElement) {
      empty.classList.toggle("hidden", presets.length > 0);
    }
  }

  // installShareQueuePresetsPage wires the Saved Queues card on
  // /share/queue. Idempotent (uses a dataset flag). Called from
  // installShareQueuePage once the page is confirmed mounted.
  function installShareQueuePresetsPage() {
    const panel = document.querySelector(`section[id="${ShareQueuePresetsSectionID}"]`);
    if (!(panel instanceof HTMLElement)) return;
    if (panel.dataset.shareQueuePresetsInstalled === "true") return;
    panel.dataset.shareQueuePresetsInstalled = "true";
    const saveForm = panel.querySelector("[data-share-queue-preset-save]");
    if (saveForm instanceof HTMLFormElement) {
      // Routes through the canonical utility-submit helper (issue
      // #317) — replaces the slice-1 marker convention.
      dispatchUtilitySubmit(saveForm, (form) => saveCurrentQueueAsPresetPage(panel, form));
    }
    refreshShareQueuePresetsPage(panel);
  }

  function installShareQueuePage() {
    const section = document.querySelector(`section[id="${ShareQueueListSectionID}"]`);
    if (!(section instanceof HTMLElement)) return;
    if (section.dataset.shareQueuePageInstalled === "true") return;
    section.dataset.shareQueuePageInstalled = "true";

    // Select-all checkbox + per-row checkboxes: delegated
    // through `section` so the handlers survive the
    // renderShareQueuePage() replaceChildren() call. The
    // select-all checkbox is inside the section, so a
    // direct addEventListener on it would be attached to
    // an element that gets detached on the next render.
    section.addEventListener("change", (ev) => {
      const target = ev.target;
      if (!(target instanceof HTMLInputElement)) return;
      if (target.matches("[data-share-queue-page-select-all]")) {
        const checked = target.checked;
        section.querySelectorAll("input[type=checkbox][data-share-queue-page-select]").forEach((el) => {
          if (el instanceof HTMLInputElement) el.checked = checked;
        });
        syncShareQueuePageButtons();
        return;
      }
      if (target.matches("input[type=checkbox][data-share-queue-page-select]")) {
        syncShareQueuePageButtons();
      }
    });

    // Per-row Remove button.
    section.addEventListener("click", (ev) => {
      const target = ev.target;
      if (!(target instanceof HTMLElement)) return;
      const remove = target.closest("[data-share-queue-page-remove-id]");
      if (remove instanceof HTMLElement) {
        const idAttr = remove.getAttribute("data-share-queue-page-remove-id");
        const id = idAttr ? parseInt(idAttr, 10) : 0;
        if (id > 0) {
          removeFromShareQueue(id);
          renderShareQueuePage();
          pageSetStatus(`Removed #${id}.`);
        }
      }
    });

    // Bulk Remove Selected.
    const bulkRemove = document.querySelector("[data-share-queue-page-bulk-remove]");
    if (bulkRemove instanceof HTMLElement) {
      bulkRemove.addEventListener("click", () => {
        const ids = getSelectedIdsOnPage();
        if (ids.length === 0) return;
        if (!window.confirm(`Remove ${ids.length} row(s) from the Share Queue?`)) return;
        const remaining = readShareQueue().filter((n) => ids.indexOf(n) === -1);
        writeShareQueue(remaining);
        renderShareQueuePage();
        pageSetStatus(`Removed ${ids.length} row(s).`);
      });
    }

    // Bulk Export: inject the selected rows into the form
    // as repeating selected_ids hidden fields before the
    // existing dispatchDixieDataForm picks up the submit.
    const exportForm = document.querySelector("[data-share-queue-page-form]");
    if (exportForm instanceof HTMLFormElement) {
      // Pre-submit hook: stage hidden fields, then the
      // data-dixie-submit dispatcher takes over. Routes through
      // the canonical submit-prep helper (issue #317) — replaces
      // the slice-1 marker convention. The helper ALLOWS the
      // submit to continue (no preventDefault), so the downstream
      // dispatch still fires.
      dispatchSubmitPrep(exportForm, (form) => {
        const ids = getSelectedIdsOnPage();
        // Drop any prior injected ids to avoid duplicates
        // (the form might be reused across multiple submits).
        const prior = form.querySelectorAll("input[type=hidden][data-share-queue-page-staged-id]");
        prior.forEach((el) => el.remove());
        for (const id of ids) {
          const inp = document.createElement("input");
          inp.type = "hidden";
          inp.name = "selected_ids";
          inp.value = String(id);
          inp.dataset.shareQueuePageStagedId = "1";
          form.appendChild(inp);
        }
      });
    }

    // Initial render: replace the server stub with the
    // localStorage-backed table.
    renderShareQueuePage();
  }

  // Live preview panel wiring (issue #179). On open we install a
  // single delegated change/input listener on the modal form so
  // rapid checkbox toggles collapse to one debounced server fetch.
  // The Refresh Preview button forces an immediate fetch. The server
  // returns an HTML fragment we inject into [data-print-config-preview].
  function installPrintConfigPreview() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const form = modal.querySelector("form");
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    if (form.dataset.printConfigPreviewInstalled === "true") {
      return;
    }
    form.dataset.printConfigPreviewInstalled = "true";
    // Issue #573: shared debounce helper. Same trailing-edge
    // semantics as the inline impl it replaced (collapse rapid
    // input/change into a single fetch after 150ms of quiet).
    // The helper is loaded by frontend/_lib/debounce.js via a
    // <script defer> in index.html that runs ahead of app.js.
    const debounce = window.__dixieDebounce;
    if (!debounce) return;
    const trigger = debounce(() => {
      refreshPrintConfigPreview();
    }, 150);
    form.addEventListener("change", trigger);
    form.addEventListener("input", trigger);
    const refreshButton = modal.querySelector("[data-print-config-preview-refresh]");
    if (refreshButton instanceof HTMLElement) {
      refreshButton.addEventListener("click", () => {
        trigger.cancel();
        refreshPrintConfigPreview();
      });
    }
  }

  async function refreshPrintConfigPreview() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const form = modal.querySelector("form");
    const target = modal.querySelector("[data-print-config-preview]");
    const status = modal.querySelector("[data-print-config-preview-status]");
    if (!(form instanceof HTMLFormElement) || !(target instanceof HTMLElement)) {
      return;
    }
    if (status instanceof HTMLElement) {
      status.textContent = "Refreshing…";
    }
    try {
      const response = await fetch("/export/preview", {
        method: "POST",
        body: new FormData(form),
      });
      if (!response.ok) {
        if (status instanceof HTMLElement) {
          status.textContent = "Preview failed.";
        }
        return;
      }
      const html = await response.text();
      target.innerHTML = html;
      if (status instanceof HTMLElement) {
        status.textContent = "";
      }
    } catch (error) {
      if (status instanceof HTMLElement) {
        status.textContent = "Preview failed.";
      }
    }
  }

  // Saved-templates wiring (issue #178). On open the dropdown is
  // populated via GET /export/templates. Load populates every
  // modal field from the JSON response. Save submits the modal's
  // full FormData plus the template_name input. Delete confirms
  // via the existing data-confirm handler then removes the row.
  function installExportTemplates() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const form = modal.querySelector("form");
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    if (form.dataset.exportTemplatesInstalled === "true") {
      return;
    }
    form.dataset.exportTemplatesInstalled = "true";
    const loadButton = modal.querySelector("[data-export-templates-load]");
    if (loadButton instanceof HTMLElement) {
      loadButton.addEventListener("click", () => {
        loadSelectedTemplate();
      });
    }
    const saveButton = modal.querySelector("[data-export-templates-save]");
    if (saveButton instanceof HTMLElement) {
      saveButton.addEventListener("click", () => {
        saveCurrentTemplate();
      });
    }
    const updateButton = modal.querySelector("[data-export-templates-update]");
    if (updateButton instanceof HTMLElement) {
      updateButton.addEventListener("click", () => {
        updateSelectedTemplate();
      });
    }
    const deleteButton = modal.querySelector("[data-export-templates-delete]");
    if (deleteButton instanceof HTMLElement) {
      deleteButton.addEventListener("click", () => {
        deleteSelectedTemplate();
      });
    }
    // Issue #184: install the inline "Show details" toggle
    // for stale-template warning lists. Click toggles the
    // <ul> visibility; the JS state is captured on the
    // wrapper's dataset so future toggles don't re-render.
    const warningsToggle = modal.querySelector("[data-export-templates-warnings-toggle]");
    const warningsWrap = modal.querySelector("[data-export-templates-warnings-wrap]");
    if (warningsToggle instanceof HTMLElement && warningsWrap instanceof HTMLElement) {
      warningsToggle.addEventListener("click", () => {
        const expanded = warningsWrap.classList.toggle("hidden");
        warningsToggle.setAttribute("aria-expanded", String(!expanded));
        warningsToggle.textContent = expanded ? "Show details" : "Hide details";
      });
    }
  }

  async function refreshExportTemplates() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const select = modal.querySelector("[data-export-templates-select]");
    const status = modal.querySelector("[data-export-templates-status]");
    if (!(select instanceof HTMLSelectElement)) {
      return;
    }
    try {
      const response = await fetch("/export/templates", { method: "GET" });
      if (!response.ok) {
        if (status instanceof HTMLElement) {
          status.textContent = "Could not load templates.";
        }
        return;
      }
      const data = await response.json();
      const templates = Array.isArray(data.templates) ? data.templates : [];
      while (select.options.length > 1) {
        select.remove(1);
      }
      for (const t of templates) {
        if (!t || typeof t.id !== "number" || typeof t.name !== "string") {
          continue;
        }
        const option = document.createElement("option");
        option.value = String(t.id);
        // Issue #186: attach the template id to the option so
        // updateSelectedTemplate() can address the right row via
        // select.selectedOptions[0].dataset.templateId.
        option.dataset.templateId = String(t.id);
        let label = t.name;
        // Issue #187: render "(N stale)" next to the option name
        // so the user sees which templates need attention before
        // clicking Load. The count comes from the LIST handler
        // (issue #187 inline approach).
        if (typeof t.stale_warning_count === "number" && t.stale_warning_count > 0) {
          label += " (" + t.stale_warning_count + " stale)";
        }
        option.textContent = label;
        select.appendChild(option);
      }
      if (status instanceof HTMLElement) {
        status.textContent = templates.length === 0 ? "No saved templates yet." : "";
      }
    } catch (error) {
      if (status instanceof HTMLElement) {
        status.textContent = "Could not load templates.";
      }
    }
  }

  async function loadSelectedTemplate() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const form = modal.querySelector("form");
    const select = modal.querySelector("[data-export-templates-select]");
    const status = modal.querySelector("[data-export-templates-status]");
    if (!(form instanceof HTMLFormElement) || !(select instanceof HTMLSelectElement)) {
      return;
    }
    const id = select.value;
    if (!id) {
      if (status instanceof HTMLElement) {
        status.textContent = "Pick a template to load.";
      }
      return;
    }
    try {
      const response = await fetch("/export/templates/" + encodeURIComponent(id) + "/apply", {
        method: "POST",
      });
      if (!response.ok) {
        if (status instanceof HTMLElement) {
          status.textContent = "Could not load template.";
        }
        return;
      }
      const envelope = await response.json();
      const template = envelope && envelope.template ? envelope.template : envelope;
      const warnings = Array.isArray(envelope && envelope.warnings) ? envelope.warnings : [];
      applyTemplateToForm(form, template);
      refreshPrintConfigPreview();
      if (status instanceof HTMLElement) {
        status.textContent = "Loaded \"" + (template.name || "") + "\".";
      }
      // Issue #186: surface the Save Changes button only after
      // a successful Load so accidental Update clicks can't
      // happen on an unselected dropdown.
      const updateBtn = modal.querySelector("[data-export-templates-update]");
      if (updateBtn instanceof HTMLElement) {
        updateBtn.classList.remove("hidden");
      }
      // Mirror the template id onto the name input so Update
      // can PATCH the right row even after the form's other
      // fields have been edited.
      const nameInput = modal.querySelector("[data-export-template-name-input]");
      if (nameInput instanceof HTMLInputElement) {
        nameInput.value = template.name || "";
      }
      // Issue #181: surface stale filter values / selected IDs as
      // toasts. One warning → one toast with detail. Many warnings
      // → one summary toast + full list in console so the user is
      // not spammed when an archive has been heavily refactored.
      if (warnings.length === 1) {
        showToast(warnings[0], "warning");
      } else if (warnings.length > 1) {
        showToast(
          warnings.length + " stale filter values; click 'Show details' for the list.",
          "warning"
        );
        console.warn("Template load warnings:", warnings);
      }

      // Issue #184: when there are ≥2 warnings, populate the
      // inline expandable list inside the modal so users
      // without devtools open can read the full text. The
      // single-warning case is the simple toast above; the
      // collapse-and-show pattern is the spec's Option A.
      const warningsWrap = modal.querySelector("[data-export-templates-warnings-wrap]");
      const warningsList = modal.querySelector("[data-export-templates-warnings]");
      const warningsToggle = modal.querySelector("[data-export-templates-warnings-toggle]");
      if (warningsWrap instanceof HTMLElement && warningsList instanceof HTMLElement) {
        while (warningsList.firstChild) warningsList.removeChild(warningsList.firstChild);
        if (warnings.length >= 2) {
          for (const w of warnings) {
            const li = document.createElement("li");
            li.textContent = w;
            warningsList.appendChild(li);
          }
          warningsWrap.classList.remove("hidden");
          if (warningsToggle instanceof HTMLElement) {
            warningsToggle.textContent = "Show details";
            warningsToggle.setAttribute("aria-expanded", "false");
          }
        } else {
          warningsWrap.classList.add("hidden");
        }
      }
    } catch (error) {
      if (status instanceof HTMLElement) {
        status.textContent = "Could not load template.";
      }
    }
  }

  /**
   * @param {HTMLFormElement} form
   * @param {{
   *   scope?: string,
   *   filters?: Record<string, string[]>,
   *   sort_by?: string,
   *   orientation?: "portrait" | "landscape",
   *   printer_friendly?: boolean,
   *   full_biography_page?: boolean,
   *   group_by?: string,
   *   group_by_unit?: boolean,
   *   group_by_pension_state?: boolean,
   *   group_by_confederate_home_status?: boolean,
   *   group_by_buried_in?: boolean,
   * }} template
   */
  function applyTemplateToForm(form, template) {
    if (!template || typeof template !== "object") {
      return;
    }
    /** @param {string} name @param {unknown} value */
    const setValue = (name, value) => {
      const element = form.elements.namedItem(name);
      if (!(element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement)) {
        return;
      }
      element.value = value == null ? "" : String(value);
    };
    /** @param {string} name @param {unknown} checked */
    const setChecked = (name, checked) => {
      const elements = form.elements.namedItem(name);
      const list = Array.isArray(elements) ? elements : elements ? [elements] : [];
      for (const el of list) {
        if (el instanceof HTMLInputElement) {
          el.checked = Boolean(checked);
        }
      }
    };
    /** @param {string} name @param {unknown} values */
    const setMultiChecked = (name, values) => {
      const elements = form.elements.namedItem(name);
      const list = Array.isArray(elements) ? elements : elements ? [elements] : [];
      const wanted = new Set(Array.isArray(values) ? values.map((v) => String(v)) : []);
      for (const el of list) {
        el.checked = wanted.has(String(el.value));
      }
    };
    if (typeof template.scope === "string") {
      const radios = form.querySelectorAll('input[name="scope"]');
      radios.forEach(/** @param {Element} r */ (r) => {
        if (r instanceof HTMLInputElement) {
          r.checked = (r.value === template.scope);
        }
      });
    }
    if (template.filters && typeof template.filters === "object") {
      for (const [family, values] of Object.entries(template.filters)) {
        setMultiChecked("filter_" + family, values);
      }
    }
    if (typeof template.sort_by === "string") {
      const radios = form.querySelectorAll('input[name="sort_by"]');
      radios.forEach(/** @param {Element} r */ (r) => {
        if (r instanceof HTMLInputElement) {
          r.checked = (r.value === template.sort_by);
        }
      });
    }
    if (typeof template.orientation === "string") {
      setValue("orientation", template.orientation);
    }
    setChecked("printer_friendly", template.printer_friendly);
    setChecked("full_biography_page", template.full_biography_page);
    const groups = new Set(Array.isArray(template.group_by) ? template.group_by : []);
    setChecked("group_by_unit", groups.has("unit"));
    setChecked("group_by_pension_state", groups.has("pension_state"));
    setChecked("group_by_confederate_home_status", groups.has("confederate_home_status"));
    setChecked("group_by_buried_in", groups.has("buried_in"));
    syncPrintScopeState();
  }

  async function saveCurrentTemplate() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const form = modal.querySelector("form");
    const nameInput = modal.querySelector("[data-export-template-name-input]");
    const status = modal.querySelector("[data-export-templates-status]");
    if (!(form instanceof HTMLFormElement) || !(nameInput instanceof HTMLInputElement)) {
      return;
    }
    const name = (nameInput.value || "").trim();
    if (!name) {
      if (status instanceof HTMLElement) {
        status.textContent = "Type a name first.";
      }
      nameInput.focus();
      return;
    }
    try {
      const response = await fetch("/export/templates", {
        method: "POST",
        body: new FormData(form),
      });
      if (response.status === 409) {
        if (status instanceof HTMLElement) {
          status.textContent = "That name is taken; pick another.";
        }
        return;
      }
      if (!response.ok) {
        if (status instanceof HTMLElement) {
          status.textContent = "Could not save template.";
        }
        return;
      }
      nameInput.value = "";
      await refreshExportTemplates();
      // Issue #234: save mutates the export templates list, which
      // is a sibling of the lazy-loaded fragment cache. Force a
      // re-fetch on next modal open so the new template name shows
      // up in the picker without a manual refresh.
      if (typeof invalidatePrintRecordsCache === "function") {
        invalidatePrintRecordsCache();
      }
      if (status instanceof HTMLElement) {
        status.textContent = "Saved.";
      }
    } catch (error) {
      if (status instanceof HTMLElement) {
        status.textContent = "Could not save template.";
      }
    }
  }

  // refreshPrintConfigTemplateDropdown re-pulls /export/templates
  // and re-populates the dropdown. Used by updateSelectedTemplate
  // to surface a renamed template in its new sort position.
  // Mirrors the install-time fetch block (kept inline because the
  // install block + later-on refresh share fetch + populate code
  // but differ in error reporting).
  async function refreshPrintConfigTemplateDropdown() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) return;
    const select = modal.querySelector("[data-export-templates-select]");
    const status = modal.querySelector("[data-export-templates-status]");
    if (!(select instanceof HTMLSelectElement)) {
      return;
    }
    try {
      const response = await fetch("/export/templates", { method: "GET" });
      if (!response.ok) {
        return;
      }
      const data = await response.json();
      const templates = Array.isArray(data.templates) ? data.templates : [];
      // Preserve the currently-selected template id across the
      // refresh so Save Changes doesn't accidentally re-init.
      const previousSelected = select.value;
      while (select.options.length > 1) {
        select.remove(1);
      }
      for (const t of templates) {
        if (!t || typeof t.id !== "number" || typeof t.name !== "string") {
          continue;
        }
        const option = document.createElement("option");
        option.value = String(t.id);
        option.dataset.templateId = String(t.id);
        let label = t.name;
        // Issue #187: stale count badge in the dropdown.
        if (typeof t.stale_warning_count === "number" && t.stale_warning_count > 0) {
          label += " (" + t.stale_warning_count + " stale)";
        }
        option.textContent = label;
        select.appendChild(option);
      }
      if (previousSelected) {
        select.value = previousSelected;
      }
      if (status instanceof HTMLElement && templates.length === 0) {
        status.textContent = "No saved templates yet.";
      }
    } catch (error) {
      // Refresh is best-effort; the user can still save with a
      // duplicate name conflict reported via the inline 409.
    }
  }

  // updateSelectedTemplate (issue #186) pushes the modal's
  // current form values back to the loaded template id. The
  // dropdown's selected option carries the template id via
  // data-template-id, set when we populated the dropdown from
  // /export/templates. The server returns {id, name} on success
  // or surfaces 404/409 inline via the existing respond-error
  // shape; the templates-status slot carries the message.
  async function updateSelectedTemplate() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) return;
    const form = modal.querySelector("#share-print-config-form");
    if (!(form instanceof HTMLFormElement)) return;
    const select = modal.querySelector("[data-export-templates-select]");
    const status = modal.querySelector("[data-export-templates-status]");
    const option = select instanceof HTMLSelectElement ? select.selectedOptions[0] : undefined;
    const templateID = option && option.dataset && option.dataset.templateId;
    if (!templateID) {
      if (status instanceof HTMLElement) {
        status.textContent = "Load a template first.";
      }
      return;
    }
    const templateNameInput = modal.querySelector("[data-export-template-name-input]");
    const fd = new FormData(form);
    if (templateNameInput instanceof HTMLInputElement && templateNameInput.value) {
      fd.set("template_name", templateNameInput.value);
    }
    try {
      const response = await fetch(`/export/templates/${encodeURIComponent(templateID)}`, {
        method: "POST", // handler accepts POST as well as PATCH
        body: fd,
      });
      /** @type {{ error?: string, name?: string }} */
      let body = {};
      try {
        body = await response.json();
      } catch (err) {
        // The server returned a non-JSON body (e.g. a Wails-internal
        // error page). Surface a modal-local error instead of
        // pretending success. Matches error-handling.md "inline
        // message" pattern for fragment targets.
        console.warn("export template update: response was not JSON", err);
        if (status instanceof HTMLElement) {
          status.textContent = "Server returned an unexpected response.";
        }
        return;
      }
      if (!response.ok) {
        if (status instanceof HTMLElement) {
          status.textContent = (body && body.error) || `Could not update template. (${response.status})`;
        }
        return;
      }
      if (status instanceof HTMLElement) {
        status.textContent = `Updated ${(body && body.name) || "template"}.`;
      }
      // Refresh the dropdown so the renamed template moves in
      // sort order if the user renamed it.
      await refreshPrintConfigTemplateDropdown();
      // Issue #234: invalidates the lazy-loaded fragment cache.
      if (typeof invalidatePrintRecordsCache === "function") {
        invalidatePrintRecordsCache();
      }
    } catch (error) {
      if (status instanceof HTMLElement) {
        status.textContent = "Could not update template.";
      }
    }
  }

  async function deleteSelectedTemplate() {
    const modal = printConfigModal();
    if (!(modal instanceof HTMLElement)) {
      return;
    }
    const select = modal.querySelector("[data-export-templates-select]");
    const status = modal.querySelector("[data-export-templates-status]");
    if (!(select instanceof HTMLSelectElement)) {
      return;
    }
    const id = select.value;
    if (!id) {
      if (status instanceof HTMLElement) {
        status.textContent = "Pick a template to delete.";
      }
      return;
    }
    try {
      const response = await fetch("/export/templates/" + encodeURIComponent(id), {
        method: "DELETE",
      });
      if (!response.ok) {
        if (status instanceof HTMLElement) {
          status.textContent = "Could not delete template.";
        }
        return;
      }
      select.value = "";
      await refreshExportTemplates();
      // Issue #234: invalidates the lazy-loaded fragment cache.
      if (typeof invalidatePrintRecordsCache === "function") {
        invalidatePrintRecordsCache();
      }
      if (status instanceof HTMLElement) {
        status.textContent = "Deleted.";
      }
    } catch (error) {
      if (status instanceof HTMLElement) {
        status.textContent = "Could not delete template.";
      }
    }
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
    const titlePresetNode = document.querySelector('input[name="title_preset"]:checked');
    const titlePreset = titlePresetNode instanceof HTMLInputElement ? titlePresetNode.value : "";
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

  function syncPrintScopeState() {
    const form = printConfigForm();
    if (!(form instanceof HTMLFormElement)) {
      return;
    }
    const scopeNode = form.querySelector('input[name="scope"]:checked');
    const scope = (scopeNode instanceof HTMLInputElement ? scopeNode.value : "all").trim();
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
    const scopeNode = form.querySelector('input[name="scope"]:checked');
    const queryNode = form.querySelector("[data-print-record-filter]");
    const scope = (scopeNode instanceof HTMLInputElement ? scopeNode.value : "all").trim();
    const query = (queryNode instanceof HTMLInputElement ? queryNode.value : "").trim().toLowerCase();
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
    const scopeNode = form.querySelector('input[name="scope"]:checked');
    const queryNode = form.querySelector("[data-print-buried-filter]");
    const scope = (scopeNode instanceof HTMLInputElement ? scopeNode.value : "all").trim();
    const query = (queryNode instanceof HTMLInputElement ? queryNode.value : "").trim().toLowerCase();
    form.querySelectorAll("[data-print-buried-option]").forEach((option) => {
      if (!(option instanceof HTMLElement)) {
        return;
      }
      const search = (option.getAttribute("data-print-buried-search") || "").toLowerCase();
      const visible = scope !== "filtered" || query === "" || search.includes(query);
      option.classList.toggle("hidden", !visible);
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    // Issue #309: install the debug toolbox onto window.dixie
    // (the toolbox is loaded as a separate script tag so it's
    // already defined by the time DOMContentLoaded fires).
    // Then unhide the floating dev badge if debug mode is on.
    if (typeof installDixieDebugToolbox === "function") {
      try {
        installDixieDebugToolbox();
      } catch (err) {
        if (typeof console !== "undefined") {
          console.warn("debug toolbox install failed", err);
        }
      }
    }
    if (window.DIXIEDATA_DEVTOOLS === true) {
      const badge = document.querySelector("[data-dixie-page-badge]");
      if (badge instanceof HTMLElement) {
        badge.classList.remove("hidden");
        badge.classList.add("inline-flex");
      }
    }

    // Option C: strip pass deleted. htmx keeps running for the GET
    // polling fragments (/jobs/active, /jobs/{id}) — it doesn't
    // double-fire because no hx-post / hx-get on a click handler
    // exists. dispatchDixieDataForm intercepts form submits and
    // data-dixie-submit / data-merge-review-action clicks.
    ensureResponsiveLayoutWatcher();
    applyResponsiveLayout(document);
    initializeTabs();
    initializeDraftForms();
    initializeArticlePreview();
    initializeEntryTypeForms();
    initializeLiveCounts(document);
    initializeFloatingNav();
    installFoldouts();
    installMegaMenus();
    installFloatingNavPanel();
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
    rememberResearchPickFromPage();
    hydrateRecentSearchResults();
    hydrateResearchPickerRecents();
    initializeBrowseView();
    flashLastMovedSourceRecord();
    // Issue #249: install the dismiss-job button handler at boot
    // so document.referrer-based navigation works on first
    // dismissal. The handler prefers the same-origin referer
    // (the page that triggered the job) and falls back to the
    // templ-provided data-dismiss-target path (the kind-specific
    // DismissTargetPath() fallback). One delegated listener
    // covers every [data-dismiss-job] on the page.
    installDismissJobButtons();
    // Issue pending: installShareQueueGlobals was previously only
    // called from openPrintConfigModal(). That meant every
    // `+ Queue` button across /browse, /soldiers/{id},
    // /review-queue, /calendar/day, /soldier-detail silently
    // no-opped until the user happened to open the print-config
    // modal first. Move the install to boot so the click handler
    // is registered before any user interaction. The functions
    // it calls (readShareQueue, writeShareQueue, pill, share-
    // queue-open trigger) are all idempotent — re-running them
    // on modal open is safe.
    installShareQueueGlobals();
    updateShareQueuePill(readShareQueue());
    // Issue #583 slice 3: paint the Activity metrics SVG line
    // graph into data-inventory-metrics-svg-host and wire the
    // legend chips. Idempotent -- the SVG host is checked for
    // presence first and the global guard __inventoryChartPainted
    // prevents double-painting on htmx swaps.
    initializeInventoryMetricsChart();
    // Re-init swapped subtrees after htmx polling swaps.
    if (typeof window !== "undefined" && window.htmx && typeof window.htmx.on === "function") {
      window.htmx.on("htmx:load", (evt) => {
        const target = evt.detail && evt.detail.elt;
        if (target instanceof HTMLElement) {
          initializeDynamicContent();
        }
      });
      // Issue #309: after a full-page swap (htmx navigates to a new
      // URL with pushUrl), refresh the breadcrumb + dev badge so
      // they reflect the new path (the server-rendered HTML for
      // the new page won't be re-rendered -- it's an in-place
      // swap). Body data-dixie-page updates so dixie.page() agrees
      // with the rendered URL.
      window.htmx.on("htmx:afterSwap", (_evt) => {
        const path = window.location.pathname || "/";
        const body = document.body;
        if (body instanceof HTMLElement) {
          body.setAttribute("data-dixie-page", path);
        }
        const badge = document.querySelector("[data-dixie-page-badge]");
        if (badge instanceof HTMLElement) {
          const pathEl = badge.querySelector("[data-dixie-page-badge-path]");
          if (pathEl) pathEl.textContent = path;
          if (window.DIXIEDATA_DEVTOOLS === true) {
            badge.classList.remove("hidden");
            badge.classList.add("inline-flex");
          }
        }
        const crumb = document.querySelector("[data-dixie-breadcrumb]");
        if (crumb instanceof HTMLElement) {
          crumb.setAttribute("data-current-path", path);
          // The server-rendered crumbs are static HTML; for now
          // we mark the path so any consumer (CSS attribute
          // selector, browser devtools) sees it. A future PR
          // could re-render crumbs client-side if needed.
        }
      });
      // The server-side blockIfFragment helper (appshell/fragment_guard.go)
      // returns 204 + X-DixieData-Redirect for htmx fragment requests during
      // blocked states (pre-mux window, setup-required, recovery, startupErr).
      // htmx does not auto-follow a 204 the way it follows a 3xx, so without
      // this listener the <body> stays empty and the user sees a white
      // screen. This handler is the single client-side bridge for the
      // fragment-204 contract; any new blocked state must rely on the same
      // header so this one hook keeps working.
      // Regression net: audit/_probe-fragment-redirect.mjs covers all four
      // blocked states via headless chromium.
      window.htmx.on("htmx:afterRequest", (evt) => {
        const xhr = evt.detail && evt.detail.xhr;
        if (!xhr || xhr.status !== 204) {
          return;
        }
        const redirect = xhr.getResponseHeader("X-DixieData-Redirect");
        if (!redirect || typeof window.location.assign !== "function") {
          return;
        }
        // Reload-on-same-path guard. Without this, a polling
        // fragment whose path is not in setupRequestAllowed
        // returns 204 + X-DixieData-Redirect: /setup while the
        // user is already on /setup — every poll triggers a full
        // reload and Chromium's IPC flood protection eventually
        // throttles navigation (see /setup mouse jitter report).
        // Same protection for any other "blocked" state.
        try {
          const target = new URL(redirect, window.location.origin);
          if (target.pathname === window.location.pathname && target.search === window.location.search) {
            return;
          }
        } catch (_) {
          // fall through to assign
        }
        window.location.assign(redirect);
      });
    }
    window.requestAnimationFrame(() => clampPopoutPanels(document));
  });

  document.addEventListener("click", (event) => {
    if (eventTargetElement(event)?.closest("summary")) {
      window.requestAnimationFrame(() => clampPopoutPanels(document));
    }
    const textMenuAction = eventTargetElement(event)?.closest("[data-text-menu-action]");
    if (textMenuAction instanceof HTMLButtonElement) {
      event.preventDefault();
      performTextContextMenuAction(textMenuAction.getAttribute("data-text-menu-action"));
      return;
    }
    const recordLink = eventTargetElement(event)?.closest("a[href]");
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
    if (!eventTargetElement(event)?.closest("#text-context-menu")) {
      closeTextContextMenu();
    }
    const externalLink = eventTargetElement(event)?.closest("a[data-open-external]");
    if (externalLink instanceof HTMLAnchorElement) {
      event.preventDefault();
      openExternalLinkInChrome(externalLink.href);
      return;
    }
    const openPrintConfig = eventTargetElement(event)?.closest("[data-print-config-open]");
    if (openPrintConfig) {
      event.preventDefault();
      openPrintConfigModal();
      return;
    }
    const openGoogleCalendarPreferences = eventTargetElement(event)?.closest("[data-google-calendar-preferences-open]");
    if (openGoogleCalendarPreferences) {
      event.preventDefault();
      openGoogleCalendarPreferencesModal();
      return;
    }
    const clearBrowseSelection = eventTargetElement(event)?.closest("[data-browse-clear-selection]");
    if (clearBrowseSelection instanceof HTMLButtonElement) {
      event.preventDefault();
      saveBrowseSelection([]);
      applyBrowseSelection(document);
      return;
    }
    const resetBrowse = eventTargetElement(event)?.closest("[data-browse-reset]");
    if (resetBrowse instanceof HTMLButtonElement) {
      event.preventDefault();
      saveBrowseState(null);
      const resetPath = resetBrowse.getAttribute("data-browse-reset-path") || "/browse";
      window.location.assign(resetPath);
      return;
    }
    const anniversaryDensityToggle = eventTargetElement(event)?.closest("[data-calendar-anniversary-density-toggle]");
    if (anniversaryDensityToggle instanceof HTMLButtonElement) {
      event.preventDefault();
      saveCalendarAnniversaryDensity(anniversaryDensityToggle.getAttribute("data-calendar-anniversary-density-toggle") || "expanded");
      applyCalendarAnniversaryDensity(document);
      return;
    }
    const layoutModeToggle = eventTargetElement(event)?.closest("[data-layout-mode-option]");
    if (layoutModeToggle instanceof HTMLButtonElement) {
      event.preventDefault();
      saveLayoutModePreference(layoutModeToggle.getAttribute("data-layout-mode-option") || "auto");
      applyResponsiveLayout(document);
      return;
    }
    const openFeedback = eventTargetElement(event)?.closest("[data-feedback-open]");
    if (openFeedback) {
      event.preventDefault();
      openFeedbackModal();
      return;
    }
    const closePrintConfig = eventTargetElement(event)?.closest("[data-print-config-close]");
    if (closePrintConfig) {
      event.preventDefault();
      closePrintConfigModal();
      return;
    }
    const closeGoogleCalendarPreferences = eventTargetElement(event)?.closest("[data-google-calendar-preferences-close]");
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
    const closeFeedback = eventTargetElement(event)?.closest("[data-feedback-close]");
    if (closeFeedback) {
      event.preventDefault();
      closeFeedbackModal();
      return;
    }
    const imageTrigger = eventTargetElement(event)?.closest("[data-image-preview]");
    if (imageTrigger) {
      event.preventDefault();
      openImageViewer(
        imageTrigger.getAttribute("data-image-preview"),
        imageTrigger.getAttribute("data-image-caption"),
        imageTrigger.getAttribute("data-image-file"),
        imageTrigger.getAttribute("data-image-preview-id"),
      );
      return;
    }
    const browseRow = eventTargetElement(event)?.closest("[data-browse-row-href]");
    if (browseRow instanceof HTMLElement && !eventTargetElement(event)?.closest("a, button, input, label, select, textarea")) {
      const href = browseRow.getAttribute("data-browse-row-href");
      if (href) {
        event.preventDefault();
        pushBackSnapshot();
        window.location.assign(href);
        return;
      }
    }
    const previewTrigger = eventTargetElement(event)?.closest("[data-preview-open]");
    if (previewTrigger instanceof HTMLElement) {
      event.preventDefault();
      openPreviewDrawer(previewTrigger.getAttribute("data-preview-target"));
      return;
    }
    if (eventTargetElement(event)?.closest("[data-preview-close],[data-preview-backdrop]")) {
      event.preventDefault();
      closePreviewDrawer();
      return;
    }
    const scratchpadOpen = eventTargetElement(event)?.closest("[data-scratchpad-open]");
    if (scratchpadOpen) {
      event.preventDefault();
      openScratchpad(scratchpadOpen);
      return;
    }
    if (eventTargetElement(event)?.closest("[data-image-rotate-ccw]")) {
      event.preventDefault();
      rotateImageViewer("ccw");
      return;
    }
    if (eventTargetElement(event)?.closest("[data-image-rotate-cw]")) {
      event.preventDefault();
      rotateImageViewer("cw");
      return;
    }
    if (eventTargetElement(event)?.closest("[data-image-zoom-in]")) {
      event.preventDefault();
      setImageViewerZoom(imageViewerState.zoom * 1.2);
      return;
    }
    if (eventTargetElement(event)?.closest("[data-image-zoom-out]")) {
      event.preventDefault();
      setImageViewerZoom(imageViewerState.zoom / 1.2);
      return;
    }
    if (eventTargetElement(event)?.closest("[data-image-reset]")) {
      event.preventDefault();
      resetImageViewerTransform();
      return;
    }
    if (eventTargetElement(event)?.closest("[data-image-screenshot]")) {
      event.preventDefault();
      saveImageViewerScreenshot();
      return;
    }
    const recordAdd = eventTargetElement(event)?.closest("[data-record-add]");
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
    const recordRemove = eventTargetElement(event)?.closest("[data-record-remove]");
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
    const clearDraftTrigger = eventTargetElement(event)?.closest("[data-clear-draft-trigger]");
    if (clearDraftTrigger instanceof HTMLElement) {
      event.preventDefault();
      showDraftDeleteConfirmation(clearDraftTrigger.closest("form"), clearDraftTrigger.getAttribute("data-clear-draft-trigger") ?? "");
      return;
    }
    const confirmClearDraft = eventTargetElement(event)?.closest("[data-confirm-clear-draft]");
    if (confirmClearDraft instanceof HTMLElement) {
      event.preventDefault();
      confirmDeleteDraftFromControl(confirmClearDraft);
      return;
    }
    const cancelClearDraft = eventTargetElement(event)?.closest("[data-cancel-clear-draft]");
    if (cancelClearDraft instanceof HTMLElement) {
      event.preventDefault();
      hideDraftDeleteConfirmation(cancelClearDraft.closest("form"), cancelClearDraft.getAttribute("data-cancel-clear-draft") ?? "");
      return;
    }
    const undoClearedDraft = eventTargetElement(event)?.closest("[data-undo-cleared-draft]");
    if (undoClearedDraft instanceof HTMLElement) {
      event.preventDefault();
      undoDeletedDraftFromControl(undoClearedDraft);
      return;
    }
    const reapplyStaleDraft = eventTargetElement(event)?.closest("[data-reapply-stale-draft]");
    if (reapplyStaleDraft instanceof HTMLElement) {
      event.preventDefault();
      reapplyStaleDraftFromControl(reapplyStaleDraft);
      return;
    }
    const imageClose = eventTargetElement(event)?.closest("[data-image-close]");
    if (imageClose || eventTargetElement(event)?.id === "image-viewer") {
      event.preventDefault();
      closeImageViewer();
      return;
    }
    const tab = eventTargetElement(event)?.closest("[data-tab-group][data-tab-target]");
    if (tab) {
      event.preventDefault();
      activateTab(tab);
      return;
    }
    const historyBack = eventTargetElement(event)?.closest("[data-history-back]");
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
    const compareSelected = eventTargetElement(event)?.closest("[data-compare-selected]");
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
    const submitTrigger = eventTargetElement(event)?.closest("[data-dixie-submit], [data-merge-review-action]");
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
    const form = eventTargetElement(event)?.closest("form[data-draft-key]");
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
    const form = eventTargetElement(event)?.closest("form[data-draft-key]");
    if (form instanceof HTMLFormElement) {
      const result = persistDraftForForm(form);
      setRecordPersistenceState(form, result.hasDraft ? "dirty" : "clean");
    }
  });
  document.addEventListener("change", (event) => {
    const pdfInput = eventTargetElement(event)?.closest("[data-pdf-pref-key]");
    if (pdfInput instanceof HTMLElement) {
      const form = pdfInput.closest("form[data-pdf-pref-scope]");
      if (form instanceof HTMLFormElement) {
        persistPDFPreferences(form);
      }
    }
  });
  document.addEventListener("change", (event) => {
    const entryTypeSelect = eventTargetElement(event)?.closest("[data-entry-type-select]");
    if (entryTypeSelect) {
      const form = entryTypeSelect.closest("form");
      if (form instanceof HTMLFormElement) {
        syncEntryTypeFields(form);
      }
    }
  });
  document.addEventListener("change", (event) => {
    const homeStatusSelect = eventTargetElement(event)?.closest("[data-confederate-home-status]");
    if (homeStatusSelect) {
      const form = homeStatusSelect.closest("form");
      if (form instanceof HTMLFormElement) {
        syncConfederateHomeFields(form);
      }
    }
  });
  // Pre-submit hook for PDF preferences persistence (issue #317
  // — was a marker-annotated raw addEventListener under slice 1).
  // Routes through the canonical submit-prep helper, which
  // ALLOWS the submit to continue (the form is a
  // data-dixie-submit form whose dispatcher will fetch a PDF).
  document.addEventListener("submit", (event) => {
    const form = event.target;
    if (form instanceof HTMLFormElement && form.matches("form[data-pdf-pref-scope]")) {
      dispatchSubmitPrep(form, (f) => persistPDFPreferences(f));
    }
  });
  document.addEventListener("change", (event) => {
    const selectAll = eventTargetElement(event)?.closest("[data-select-all]");
    if (!(selectAll instanceof HTMLInputElement)) {
      return;
    }
    toggleCheckboxGroup(selectAll.getAttribute("data-select-all") ?? "", selectAll.checked);
  });
  document.addEventListener("change", (event) => {
    const browseFilter = eventTargetElement(event)?.closest("[data-browse-filter-input]");
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
        // Issue #573: collapse the clearTimeout/setTimeout pair into
        // the shared debounce helper. Same trailing-edge semantics
        // (one fetch per 200ms of quiet). The instance is stashed on
        // `window` so re-mounts of the form keep the same timer and
        // the freshest URL/selector/form are passed via arguments to
        // the trailing fire.
        /** @type {string} */
        const url = form.getAttribute("hx-get") || form.action || "";
        /** @type {string} */
        const targetSelector = form.getAttribute("hx-target") || "#browse-results";
        if (!url) { return; }
        const debounce = window.__dixieDebounce;
        if (!debounce) { return; }
        if (typeof window.__dixieBrowseFilterDebounce !== "function") {
          window.__dixieBrowseFilterDebounce = debounce((latestForm, latestUrl, latestTarget) => {
            (async () => {
              const params = new URLSearchParams(Array.from(new FormData(latestForm).entries(), ([k, v]) => [k, typeof v === "string" ? v : ""]));
              try {
                const response = await fetch(`${latestUrl}?${params.toString()}`, {
                  method: "GET",
                  headers: { "X-Requested-With": "DixieData" },
                });
                const html = await response.text();
                const target = document.querySelector(latestTarget);
                if (target instanceof HTMLElement) {
                  target.innerHTML = html;
                  initializeDynamicContent();
                }
              } catch (error) {
                showToast("Browse refresh failed.", "error");
              }
            })();
          }, 200);
        }
        window.__dixieBrowseFilterDebounce(form, url, targetSelector);
      }
      return;
    }
    const browseColumnToggle = eventTargetElement(event)?.closest("[data-browse-column-toggle]");
    if (browseColumnToggle instanceof HTMLInputElement) {
      const enabled = [];
      for (const input of document.querySelectorAll("[data-browse-column-toggle]")) {
        if (input instanceof HTMLInputElement && input.checked) {
          enabled.push(input.value);
        }
      }
      saveBrowseColumns(enabled);
      applyBrowseColumns(document);
      return;
    }
    const browseSelect = eventTargetElement(event)?.closest("[data-browse-select]");
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
    const compareSelect = eventTargetElement(event)?.closest("[data-compare-select]");
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
    const editableTarget = eventTargetElement(event)?.closest("input, textarea, [contenteditable='true'], [contenteditable=''], [contenteditable='plaintext-only']");
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
    const stage = eventTargetElement(event)?.closest("[data-image-stage]");
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
      if (eventTargetElement(event)?.tagName === "DETAILS") {
        window.requestAnimationFrame(() => clampPopoutPanels(document));
      }
    },
    true,
  );
  document.addEventListener(
    "wheel",
    (event) => {
      const stage = eventTargetElement(event)?.closest("[data-image-stage]");
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
