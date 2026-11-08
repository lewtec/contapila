/* Collapsible account tree (P&L / net worth) — expanded by default.
 * data-account is the tree path; descendants match path+":" or path+"\x1f".
 */
(function () {
  "use strict";

  var CC_SEP = "\x1f"; // multi-commodity under an account

  function isDescendant(path, parent) {
    if (!path || !parent || path === parent) return false;
    return path.indexOf(parent + ":") === 0 || path.indexOf(parent + CC_SEP) === 0;
  }

  function refresh(table) {
    var collapsed = [];
    table.querySelectorAll("tr.pnl-collapsed[data-account]").forEach(function (tr) {
      collapsed.push(tr.getAttribute("data-account"));
    });
    table.querySelectorAll("tr[data-account]").forEach(function (row) {
      var a = row.getAttribute("data-account");
      var hide = false;
      for (var i = 0; i < collapsed.length; i++) {
        if (isDescendant(a, collapsed[i])) {
          hide = true;
          break;
        }
      }
      row.hidden = hide;
    });
  }

  function bind(table) {
    table.addEventListener("click", function (e) {
      var btn = e.target.closest("[data-pnl-toggle]");
      if (!btn || !table.contains(btn)) return;
      e.preventDefault();
      var tr = btn.closest("tr");
      if (!tr) return;
      var collapsed = tr.classList.toggle("pnl-collapsed");
      btn.textContent = collapsed ? "▶" : "▼";
      btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
      btn.setAttribute("title", collapsed ? "Expand" : "Collapse");
      refresh(table);
    });
  }

  function init() {
    document.querySelectorAll("[data-pnl-tree]").forEach(bind);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
