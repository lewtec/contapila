/* Command palette: filter SSR list, keyboard nav, ⌘K / Ctrl+K open.
 * Expects #cmd-palette (dialog), #cmd-palette-input, #cmd-palette-list [data-cmd-item],
 * #cmd-palette-open, #cmd-palette-empty. Optional #cmd-palette-hotkey for platform label.
 */
(function () {
  "use strict";

  var dialog, input, list, emptyEl, openBtn;
  var items = [];
  var visible = [];
  var active = -1;

  function qs(sel, root) {
    return (root || document).querySelector(sel);
  }

  function collect() {
    items = Array.prototype.slice.call(list.querySelectorAll("[data-cmd-item]"));
  }

  function setActive(idx) {
    if (visible.length === 0) {
      active = -1;
      input && input.setAttribute("aria-activedescendant", "");
      return;
    }
    if (idx < 0) idx = visible.length - 1;
    if (idx >= visible.length) idx = 0;
    active = idx;
    for (var i = 0; i < visible.length; i++) {
      var el = visible[i];
      var on = i === active;
      el.setAttribute("aria-selected", on ? "true" : "false");
      el.classList.toggle("cmd-active", on);
      if (on) {
        var id = el.id || "cmd-item-" + i;
        el.id = id;
        input.setAttribute("aria-activedescendant", id);
        el.scrollIntoView({ block: "nearest" });
      }
    }
  }

  function filter() {
    var q = (input.value || "").trim().toLowerCase();
    var tokens = q ? q.split(/\s+/).filter(Boolean) : [];
    visible = [];
    for (var i = 0; i < items.length; i++) {
      var el = items[i];
      var hay = (el.getAttribute("data-keywords") || "").toLowerCase();
      var ok = true;
      for (var t = 0; t < tokens.length; t++) {
        if (hay.indexOf(tokens[t]) === -1) {
          ok = false;
          break;
        }
      }
      el.hidden = !ok;
      if (ok) visible.push(el);
    }
    if (emptyEl) {
      var none = visible.length === 0;
      emptyEl.hidden = !none;
      emptyEl.classList.toggle("hidden", !none);
    }
    setActive(visible.length ? 0 : -1);
  }

  function open() {
    if (!dialog || typeof dialog.showModal !== "function") return;
    if (dialog.open) return;
    collect();
    input.value = "";
    filter();
    try {
      dialog.showModal();
    } catch (err) {
      // Fallback if already open or not allowed.
      if (!dialog.open) dialog.setAttribute("open", "");
    }
    requestAnimationFrame(function () {
      try {
        input.focus();
        input.select();
      } catch (e) {}
    });
  }

  function close() {
    if (!dialog) return;
    if (typeof dialog.close === "function" && dialog.open) {
      dialog.close();
      return;
    }
    dialog.removeAttribute("open");
  }

  function activate() {
    if (active < 0 || active >= visible.length) return;
    var a = visible[active].querySelector("a");
    if (a && a.href) {
      window.location.href = a.href;
    }
  }

  function isOpenHotkey(e) {
    if (!(e.metaKey || e.ctrlKey)) return false;
    // code is layout-stable; key covers older engines
    return e.code === "KeyK" || e.key === "k" || e.key === "K";
  }

  function onKeydown(e) {
    if (isOpenHotkey(e)) {
      e.preventDefault();
      e.stopPropagation();
      if (dialog && dialog.open) close();
      else open();
      return;
    }
    if (!dialog || !dialog.open) return;

    if (e.key === "Escape") {
      e.preventDefault();
      close();
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive(active + 1);
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive(active - 1);
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      activate();
    }
  }

  function labelHotkey() {
    var el = qs("#cmd-palette-hotkey");
    if (!el) return;
    var isMac = /Mac|iPhone|iPad|iPod/.test(navigator.platform || "") ||
      (navigator.userAgentData && navigator.userAgentData.platform === "macOS");
    el.textContent = isMac ? "⌘K" : "Ctrl+K";
  }

  function init() {
    dialog = qs("#cmd-palette");
    input = qs("#cmd-palette-input");
    list = qs("#cmd-palette-list");
    emptyEl = qs("#cmd-palette-empty");
    openBtn = qs("#cmd-palette-open");
    if (!dialog || !input || !list) return;

    labelHotkey();
    collect();
    input.addEventListener("input", filter);
    // Capture so Ctrl+K wins over browser chrome / focused inputs where possible.
    document.addEventListener("keydown", onKeydown, true);
    if (openBtn) {
      openBtn.addEventListener("click", function (e) {
        e.preventDefault();
        open();
      });
    }
    list.addEventListener("mousemove", function (e) {
      var li = e.target.closest("[data-cmd-item]");
      if (!li || li.hidden) return;
      var idx = visible.indexOf(li);
      if (idx >= 0 && idx !== active) setActive(idx);
    });
    // Backdrop click (native dialog): click on dialog itself, not panel.
    dialog.addEventListener("click", function (e) {
      if (e.target === dialog) close();
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
