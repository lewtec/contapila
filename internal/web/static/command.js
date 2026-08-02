/* Command palette: filter SSR list, keyboard nav, ⌘K / Ctrl+K open.
 * Expects #cmd-palette (dialog), #cmd-palette-input, #cmd-palette-list [data-cmd-item],
 * #cmd-palette-open, #cmd-palette-empty.
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
    if (!dialog || dialog.open) return;
    input.value = "";
    filter();
    dialog.showModal();
    // Focus after paint so dialog is interactive.
    requestAnimationFrame(function () {
      input.focus();
      input.select();
    });
  }

  function close() {
    if (!dialog || !dialog.open) return;
    dialog.close();
  }

  function activate() {
    if (active < 0 || active >= visible.length) return;
    var a = visible[active].querySelector("a");
    if (a && a.href) {
      window.location.href = a.href;
    }
  }

  function onKeydown(e) {
    var mod = e.metaKey || e.ctrlKey;
    if (mod && (e.key === "k" || e.key === "K")) {
      e.preventDefault();
      if (dialog.open) close();
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

  function init() {
    dialog = qs("#cmd-palette");
    input = qs("#cmd-palette-input");
    list = qs("#cmd-palette-list");
    emptyEl = qs("#cmd-palette-empty");
    openBtn = qs("#cmd-palette-open");
    if (!dialog || !input || !list) return;

    collect();
    input.addEventListener("input", filter);
    document.addEventListener("keydown", onKeydown);
    if (openBtn) {
      openBtn.addEventListener("click", function (e) {
        e.preventDefault();
        open();
      });
    }
    // Click row: allow default navigation (href).
    list.addEventListener("mousemove", function (e) {
      var li = e.target.closest("[data-cmd-item]");
      if (!li || li.hidden) return;
      var idx = visible.indexOf(li);
      if (idx >= 0 && idx !== active) setActive(idx);
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
