/* cmdpalette: filter SSR list, keyboard nav, ⌘K / Ctrl+K.
 * Binds every dialog[data-cmdpalette]. IDs: {id}, {id}-open, {id}-input,
 * {id}-list, {id}-empty, optional {id}-hotkey.
 */
(function () {
  "use strict";
  if (window.__cmdpaletteBound) return;
  window.__cmdpaletteBound = true;

  function qs(sel, root) {
    return (root || document).querySelector(sel);
  }

  function bind(dialog) {
    var id = dialog.id;
    if (!id) return;
    var input = document.getElementById(id + "-input");
    var list = document.getElementById(id + "-list");
    var emptyEl = document.getElementById(id + "-empty");
    var openBtn = document.getElementById(id + "-open");
    var hotkeyEl = document.getElementById(id + "-hotkey");
    if (!input || !list) return;

    var items = [];
    var visible = [];
    var active = -1;

    function collect() {
      items = Array.prototype.slice.call(list.querySelectorAll("[data-cmd-item]"));
    }

    function setActive(idx) {
      if (visible.length === 0) {
        active = -1;
        input.setAttribute("aria-activedescendant", "");
        return;
      }
      if (idx < 0) idx = visible.length - 1;
      if (idx >= visible.length) idx = 0;
      active = idx;
      for (var i = 0; i < visible.length; i++) {
        var el = visible[i];
        var on = i === active;
        el.setAttribute("aria-selected", on ? "true" : "false");
        el.classList.toggle("cmdpalette-active", on);
        if (on) {
          var eid = el.id || id + "-item-" + i;
          el.id = eid;
          input.setAttribute("aria-activedescendant", eid);
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
        emptyEl.classList.toggle("cmdpalette-hidden", !none);
      }
      setActive(visible.length ? 0 : -1);
    }

    function open() {
      if (typeof dialog.showModal !== "function") return;
      if (dialog.open) return;
      collect();
      input.value = "";
      filter();
      try {
        dialog.showModal();
      } catch (err) {
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
      if (typeof dialog.close === "function" && dialog.open) {
        dialog.close();
        return;
      }
      dialog.removeAttribute("open");
    }

    function activate() {
      if (active < 0 || active >= visible.length) return;
      var a = visible[active].querySelector("a");
      if (a && a.href) window.location.href = a.href;
    }

    function isOpenHotkey(e) {
      if (!(e.metaKey || e.ctrlKey)) return false;
      return e.code === "KeyK" || e.key === "k" || e.key === "K";
    }

    function onKeydown(e) {
      if (isOpenHotkey(e)) {
        e.preventDefault();
        e.stopPropagation();
        if (dialog.open) close();
        else open();
        return;
      }
      if (!dialog.open) return;
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

    if (hotkeyEl) {
      var isMac =
        /Mac|iPhone|iPad|iPod/.test(navigator.platform || "") ||
        (navigator.userAgentData && navigator.userAgentData.platform === "macOS");
      hotkeyEl.textContent = isMac ? "⌘K" : "Ctrl+K";
    }

    collect();
    input.addEventListener("input", filter);
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
    dialog.addEventListener("click", function (e) {
      if (e.target === dialog) close();
    });
  }

  function init() {
    var nodes = document.querySelectorAll("dialog[data-cmdpalette]");
    for (var i = 0; i < nodes.length; i++) bind(nodes[i]);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
