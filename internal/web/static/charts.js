/* contapila chart glue (uPlot). Expects global uPlot. */
(function (global) {
  "use strict";

  function cssVar(name, fallback) {
    try {
      var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
      return v || fallback;
    } catch (e) {
      return fallback;
    }
  }

  function colors() {
    return {
      primary: cssVar("--color-primary", "#1a5632"),
      income: cssVar("--color-success", "#2d6a4f"),
      expense: cssVar("--color-error", "#b91c1c"),
      grid: cssVar("--color-base-300", "#e5e7eb"),
      text: cssVar("--color-base-content", "#1f2937"),
    };
  }

  /** Full width of the chart host (not the .uplot min-content box). */
  function measureWidth(el) {
    var w = 0;
    if (el) {
      w = el.clientWidth;
      if (w < 32 && el.parentElement) w = el.parentElement.clientWidth;
    }
    return w >= 32 ? w : Math.max(320, (document.documentElement.clientWidth || 640) - 48);
  }

  function fmtMoney(_u, v) {
    if (v == null || Number.isNaN(v)) return "";
    return v.toLocaleString(undefined, { maximumFractionDigits: 2, minimumFractionDigits: 0 });
  }

  /** Compact Y ticks so the left gutter stays readable (legend keeps full money). */
  function fmtMoneyAxis(_u, vals) {
    return vals.map(function (v) {
      if (v == null || Number.isNaN(v)) return "";
      var a = Math.abs(v);
      if (a >= 1e6) return (v / 1e6).toLocaleString(undefined, { maximumFractionDigits: 1 }) + "M";
      if (a >= 1e4) return (v / 1e3).toLocaleString(undefined, { maximumFractionDigits: 0 }) + "k";
      return v.toLocaleString(undefined, { maximumFractionDigits: 0 });
    });
  }

  /** Calendar date only (UTC) — no hours. */
  function fmtDate(ts) {
    if (ts == null || Number.isNaN(ts)) return "";
    var d = new Date(ts * 1000);
    var y = d.getUTCFullYear();
    var m = d.getUTCMonth() + 1;
    var day = d.getUTCDate();
    return y + "-" + (m < 10 ? "0" : "") + m + "-" + (day < 10 ? "0" : "") + day;
  }

  function fmtDateAxis(_u, splits) {
    return splits.map(fmtDate);
  }

  /**
   * How many ordinal indices between labeled ticks so each label has ~minPx of width.
   * Data series stay full length — only axis splits thin out.
   */
  function labelStride(n, chartW, minPx) {
    if (n <= 1) return 1;
    var maxLabels = Math.max(2, Math.floor(chartW / minPx));
    return Math.max(1, Math.ceil(n / maxLabels));
  }

  /**
   * Pick ordinal split indices: every `stride`, always include first and last.
   * Used as uPlot axes[0].splits so all bars remain; only ticks/labels subsample.
   * If the final stride tick sits too close to the last bin, replace it with last
   * (avoids "2026-03" + "2026-06" jammed at the end).
   */
  function ordinalLabelSplits(n, stride) {
    if (n <= 0) return [];
    if (stride <= 1) {
      var all = [];
      for (var i = 0; i < n; i++) all.push(i);
      return all;
    }
    var out = [];
    var last = n - 1;
    for (var j = 0; j < n; j += stride) out.push(j);
    if (out[out.length - 1] !== last) {
      if (last - out[out.length - 1] < stride) out[out.length - 1] = last;
      else out.push(last);
    }
    return out;
  }

  function baseOpts(el, theme, height) {
    return {
      width: measureWidth(el),
      height: height || 192,
      // [top, right, bottom, left] — left padding so Y labels aren't flush-clipped
      padding: [8, 12, 0, 12],
      cursor: { show: true, points: { show: true } },
      legend: { show: true },
      scales: { x: { time: true }, y: {} },
      axes: [
        {
          stroke: theme.text,
          grid: { stroke: theme.grid, width: 1 },
          ticks: { stroke: theme.grid },
          values: fmtDateAxis,
          size: 40,
          gap: 4,
          // Prefer fewer time ticks over rotated dense labels (uPlot default space is 50).
          space: 72,
        },
        {
          stroke: theme.text,
          grid: { stroke: theme.grid, width: 1 },
          ticks: { stroke: theme.grid },
          // Reserve enough horizontal room for money ticks (default ~50 clips them)
          size: 72,
          gap: 8,
          values: fmtMoneyAxis,
        },
      ],
    };
  }

  // series[0] = x; default uPlot legend shows "Time: date hour" — date only.
  function xSeries(labels) {
    return {
      label: "Date",
      value: function (_u, raw, _s, idx) {
        if (raw == null) return "";
        if (labels && idx != null && labels[idx] != null) return labels[idx];
        return fmtDate(raw);
      },
    };
  }

  function makeLine(el, payload) {
    var theme = colors();
    var xs = payload.x || [];
    var ys = payload.y || [];
    if (!xs.length) {
      el.innerHTML = '<p class="text-xs text-base-content/60 p-2">No series data.</p>';
      return null;
    }
    var opts = baseOpts(el, theme, payload.height);
    // Full series always; space on the X axis (baseOpts) thins date ticks only.
    opts.series = [
      xSeries(null),
      {
        label: payload.label || "Value",
        stroke: theme.primary,
        width: 2,
        fill: "transparent",
        points: { show: xs.length < 80 },
        // Smooth cubic between event samples (vs stepped holds). Data points unchanged.
        paths: uPlot.paths.spline(),
        value: function (_u, v) {
          return fmtMoney(null, v) + (payload.currency ? " " + payload.currency : "");
        },
      },
    ];
    return new uPlot(opts, [xs, ys], el);
  }

  function makeBars(el, payload) {
    var theme = colors();
    // Ordinal bins (0..n-1) — not unix time. Time scale made bars look stacked/smeared.
    var n = (payload.labels && payload.labels.length) || (payload.income && payload.income.length) || 0;
    var xs = [];
    for (var i = 0; i < n; i++) xs.push(i);
    var labels = payload.labels || [];
    var inc = payload.income || [];
    var exp = (payload.expense || []).map(function (v) {
      // Per-bin magnitude only; plot expenses below zero (not cumulative).
      return v == null ? null : -Math.abs(v);
    });
    if (!xs.length) {
      el.innerHTML = '<p class="text-xs text-base-content/60 p-2">No series data.</p>';
      return null;
    }
    var opts = baseOpts(el, theme, payload.height || 200);
    opts.scales = {
      x: { time: false, range: [-0.5, n - 0.5] },
      y: {},
    };
    // All bars stay in the series; only axis splits/labels are subsampled.
    var chartW = measureWidth(el);
    var stride = labelStride(n, chartW, 72); // ~YYYY-MM + gap
    opts.axes[0].splits = function () {
      return ordinalLabelSplits(n, stride);
    };
    opts.axes[0].values = function (_u, splits) {
      return splits.map(function (s) {
        var i = Math.round(s);
        if (i < 0 || i >= labels.length) return "";
        return labels[i];
      });
    };
    opts.axes[0].rotate = 0;
    opts.axes[0].size = 40;
    opts.axes[0].gap = 4;
    opts.series = [
      {
        label: "Period",
        value: function (_u, _raw, _s, idx) {
          if (idx == null || idx < 0 || idx >= labels.length) return "";
          return labels[idx];
        },
      },
      {
        label: "Income",
        stroke: theme.income,
        fill: theme.income,
        paths: uPlot.paths.bars({ size: [0.7, 100], align: 0 }),
        points: { show: false },
        value: function (_u, v) {
          return fmtMoney(null, v) + (payload.currency ? " " + payload.currency : "");
        },
      },
      {
        label: "Expenses",
        stroke: theme.expense,
        fill: theme.expense,
        paths: uPlot.paths.bars({ size: [0.7, 100], align: 0 }),
        points: { show: false },
        value: function (_u, v) {
          return fmtMoney(null, v == null ? null : Math.abs(v)) + (payload.currency ? " " + payload.currency : "");
        },
      },
    ];
    return new uPlot(opts, [xs, inc, exp], el);
  }

  function mount(elOrId, payload) {
    var el = typeof elOrId === "string" ? document.getElementById(elOrId) : elOrId;
    if (!el || !payload || typeof uPlot === "undefined") return null;
    el.innerHTML = "";
    var plot;
    if (payload.kind === "bars") plot = makeBars(el, payload);
    else plot = makeLine(el, payload);

    if (!plot) return null;

    function syncSize() {
      // Measure the host, not the .uplot min-content child.
      var w = measureWidth(el);
      if (w > 0 && plot) plot.setSize({ width: w, height: plot.height });
    }
    // Next frame: layout settled (fonts, sidebar).
    requestAnimationFrame(syncSize);
    window.addEventListener("resize", syncSize);

    var themeBox = document.getElementById("theme-toggle");
    if (themeBox) {
      themeBox.addEventListener("change", function () {
        if (plot) {
          plot.destroy();
          plot = null;
        }
        el.innerHTML = "";
        if (payload.kind === "bars") plot = makeBars(el, payload);
        else plot = makeLine(el, payload);
        requestAnimationFrame(syncSize);
      });
    }
    return plot;
  }

  function bootFromScript(id) {
    var el = document.getElementById(id);
    var dataEl = document.getElementById(id + "-data");
    if (!el || !dataEl) return;
    try {
      var payload = JSON.parse(dataEl.textContent);
      mount(el, payload);
    } catch (e) {
      el.innerHTML = '<p class="text-xs text-error p-2">Chart data error.</p>';
    }
  }

  global.contapilaChart = { mount: mount, boot: bootFromScript };
})(window);
