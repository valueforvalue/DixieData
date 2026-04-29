(() => {
  const timers = new WeakMap();

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

  function activateTab(trigger) {
    const group = trigger.getAttribute("data-tab-group");
    const targetId = trigger.getAttribute("data-tab-target");
    if (!group || !targetId) {
      return;
    }

    document.querySelectorAll(`[data-tab-group="${group}"]`).forEach((button) => {
      const active = button === trigger;
      button.classList.toggle("bg-yellow-900", active);
      button.classList.toggle("text-white", active);
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
    const options = {
      method,
      headers: {
        "X-Requested-With": "DixieData"
      }
    };

    let requestUrl = url;
    if (method === "GET") {
      const params = toSearchParams(data);
      const query = params.toString();
      if (query) {
        requestUrl += (url.includes("?") ? "&" : "?") + query;
      }
    } else {
      if (hasFiles(data)) {
        options.body = data;
      } else {
        options.headers["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8";
        options.body = toSearchParams(data).toString();
      }
    }

    const response = await fetch(requestUrl, options);
    const html = await response.text();
    applyResponse(el, html);
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

  document.addEventListener("DOMContentLoaded", () => {
    initializeTabs();
    document.querySelectorAll('[hx-trigger="load"]').forEach((el) => {
      request(el);
    });
  });

  document.addEventListener("click", (event) => {
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
    if (!form.hasAttribute("hx-post") && !form.hasAttribute("hx-put")) {
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
})();
