(() => {
  "use strict";
  if (!window.StroppyChart || !window.ResizeObserver) return;

  const number = new Intl.NumberFormat("en", { maximumFractionDigits: 3 });
  const compact = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 2 });
  const format = value => Number.isFinite(value) ? (Math.abs(value) >= 10000 ? compact : number).format(value) : "—";
  const states = new Set();
  const theme = window.matchMedia("(prefers-color-scheme: light)");

  const isBars = source => source.kind === "bars" || source.kind === "grouped-bars";

  function model(source) {
    if (source.kind === "grouped-bars") {
      if (!source.labels.length || source.labels.length > 16) throw new Error("dense categories retain static chart");
      const runs = source.series.length;
      const x = source.labels.flatMap((_, category) => source.series.map((_, run) =>
        category + (run - (runs - 1) / 2) * 0.75 / runs));
      const tables = source.series.map((series, run) => {
        if (series.bar_values.length !== source.labels.length) throw new Error("invalid grouped bars");
        return x.map((_, index) => {
          const value = series.bar_values[Math.floor(index / runs)];
          if (value != null && !Number.isFinite(value)) throw new Error("invalid grouped value");
          return index % runs === run ? value : null;
        });
      });
      return { data: [x, ...tables], series: source.series.map(series => ({ title: series.title, unit: source.unit, points: [] })) };
    }
    if (source.kind === "bars") {
      if (source.labels.length > 16) throw new Error("dense categories retain static chart");
      const values = source.series[0].values;
      if (!values.length || values.some(value => !Number.isFinite(value))) throw new Error("invalid bars");
      return {
        data: [values.map((_, i) => i), values],
        series: source.series.map(series => ({ title: series.title, unit: source.unit, points: [] }))
      };
    }
    const clock = new Set();
    const tables = source.series.map(series => {
      const values = new Map();
      let previous;
      for (const point of series.points || []) {
        const x = Number(point.time), y = Number(point.value);
        if (!Number.isFinite(x) || !Number.isFinite(y) || x < 0 || (previous && x <= previous.x)) {
          throw new Error("invalid timeline clock");
        }
        if (previous && (series.omitted || point.sequence !== previous.sequence + 1 || x - previous.x > series.interval * 1.8)) {
          const gap = previous.x + (x - previous.x) / 2;
          clock.add(gap);
          values.set(gap, null);
        }
        clock.add(x);
        values.set(x, y);
        previous = { x, sequence: point.sequence };
      }
      return values;
    });
    const x = [...clock].sort((a, b) => a - b);
    if (x.length < 2) throw new Error("not enough timeline samples");
    return {
      data: [x, ...tables.map(values => x.map(time => values.has(time) ? values.get(time) : undefined))],
      series: source.series.map(series => ({ title: series.title, unit: source.unit, points: [] }))
    };
  }

  function tooltip(source) {
    let tip;
    return { hooks: {
      init: [plot => {
        tip = document.createElement("div");
        tip.className = "st-chart-tooltip report-chart-tooltip";
        tip.hidden = true;
        plot.over.appendChild(tip);
        plot.over.addEventListener("mouseleave", () => { tip.hidden = true; });
      }],
      setCursor: [plot => {
        const index = plot.cursor.idx;
        if (index == null || plot.cursor.left < 0) { tip.hidden = true; return; }
        if (source.series.every((_, i) => plot.data[i + 1][index] == null)) {
          tip.hidden = true;
          return;
        }
        const x = plot.data[0][index];
        const heading = document.createElement("div");
        heading.className = "st-chart-tooltip-time";
        heading.textContent = isBars(source) ? source.labels[Math.round(x)] : `${format(x)} seconds elapsed`;
        const rows = source.series.map((series, i) => {
          const row = document.createElement("div");
          row.className = "st-chart-tooltip-row";
          const swatch = document.createElement("i");
          swatch.style.background = getComputedStyle(plot.root).getPropertyValue(`--st-series-${series.color || 1}`).trim();
          const name = document.createElement("span");
          name.textContent = series.title;
          const value = document.createElement("b");
          const recorded = source.kind === "grouped-bars" ? series.bar_values[Math.round(x)] : plot.data[i + 1][index];
          value.textContent = recorded == null ? "—" : `${format(recorded)} ${source.unit}`;          if (recorded != null) value.title = `${String(recorded)} ${source.unit}`;
          row.append(swatch, name, value);
          return row;
        });
        const events = source.series.flatMap(series => (series.markers || [])
          .filter(marker => Math.abs(marker.elapsed - x) <= series.interval)
          .map(marker => `${series.title}: ${marker.label}`));
        const event = document.createElement("div");
        event.textContent = events.join(" · ");
        tip.replaceChildren(heading, ...rows, event);
        tip.hidden = false;
        tip.style.left = `${Math.max(0, Math.min(plot.cursor.left + 12, plot.over.clientWidth - tip.offsetWidth))}px`;
        tip.style.top = "8px";
      }]
    } };
  }

  function annotations(source, target) {
    return plot => {
      const context = plot.ctx;
      context.save();
      context.beginPath();
      context.rect(plot.bbox.left, plot.bbox.top, plot.bbox.width, plot.bbox.height);
      context.clip();
      context.strokeStyle = getComputedStyle(target).getPropertyValue("--st-line-strong").trim();
      context.lineWidth = devicePixelRatio;
      for (const series of source.series) {
        for (const marker of series.markers || []) {
          const x = plot.valToPos(marker.elapsed, "x", true);
          context.beginPath();
          context.moveTo(x, plot.bbox.top);
          context.lineTo(x, plot.bbox.top + plot.bbox.height);
          context.stroke();
        }
      }
      context.restore();
    };
  }

  function restore(state) {
    state.figure.classList.remove("chart-enhanced");
    state.target.hidden = true;
    state.toggle.hidden = true;
    try {
      if (state.handle) state.handle.destroy();
      else state.plot?.destroy();
    } finally {
      state.handle = null;
      state.plot = null;
      state.target.replaceChildren();
    }
  }

  function mount(state) {
    const { source, target, figure } = state;
    target.hidden = false;
    let drawn;
    const ready = new Promise(resolve => { drawn = resolve; });
    const aligned = model(source);
    state.handle = StroppyChart.mount(target, aligned.series, {
      height: isBars(source) ? 300 : 240,
      unit: source.unit,
      data: aligned.data,
      configure(options, Plot) {
        options.scales.x = { time: false };
        options.axes[0].values = (_, ticks) => ticks.map(value => isBars(source) ?
          (source.labels[value] || "") : `${format(value)} s`);
        options.axes[1].values = (_, ticks) => ticks.map(format);
        if (isBars(source)) {
          options.scales.x.range = () => [-0.5, source.labels.length - 0.5];
          options.scales.y = { range: (_, min, max) => [0, Math.max(1, max * 1.05)] };
          options.axes[0].splits = () => source.labels.map((_, i) => i);
          options.axes[0].size = 48;          options.axes[0].values = (_, ticks) => ticks.map(value => {
            const label = source.labels[value] || "";
            const width = target.clientWidth || 600;
            const limit = Math.max(3, Math.min(18, Math.floor((width - 56) / source.labels.length / 7)));
            if (label === "Median / p50" && limit < label.length) return "p50";
            if (label === "Average" && limit < label.length) return "Avg";
            return label.length > limit ? `${label.slice(0, limit - 1)}…` : label;          });
        }
        options.series.slice(1).forEach((series, i) => {
          const original = source.series[i];
          const color = getComputedStyle(target).getPropertyValue(`--st-series-${original.color || 1}`).trim();
          Object.assign(series, { stroke: color, fill: null, paths: Plot.paths.linear(), spanGaps: false,
            width: 2, dash: original.pattern === "none" ? [] : (original.pattern || "").split(" ").filter(Boolean).map(Number),
            points: { show: true, size: 6, fill: color, stroke: color } });
          if (isBars(source)) Object.assign(series, {
            fill: color, paths: Plot.paths.bars({ size: [0.6, 60] }), points: { show: false }
          });
        });
        options.plugins = [tooltip(source)];
        options.hooks = { ready: [plot => drawn(plot)], draw: [annotations(source, target)] };
      },
      mounted(plot) { state.plot = plot; },
      onError() { state.failed = true; restore(state); }
    });
    let timer;
    Promise.race([ready, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error("chart mount timed out")), 3000);
    })]).then(() => {
      if (!target.querySelector("canvas") || !state.handle) throw new Error("chart did not mount");
      figure.classList.add("chart-enhanced");
      state.toggle.textContent = "Show static chart";
      state.toggle.hidden = false;
    }).catch(() => { state.failed = true; restore(state); }).finally(() => clearTimeout(timer));
  }

  function enhance(figure) {
    const target = document.createElement("div");
    target.className = "chart-interactive";
    target.setAttribute("role", "img");
    target.setAttribute("aria-label", figure.querySelector("h3")?.textContent || "Interactive chart");
    target.hidden = true;
    const toggle = document.createElement("button");
    toggle.className = "chart-toggle";
    toggle.textContent = "Show static chart";
    toggle.hidden = true;
    const fallback = figure.querySelector(":scope > .chart-scroll");
    if (!fallback) return;
    fallback.before(target, toggle);
    const state = { figure, target, toggle, source: JSON.parse(figure.dataset.chart), handle: null };
    states.add(state);
    toggle.addEventListener("click", () => {
      const active = figure.classList.toggle("chart-enhanced");
      target.hidden = !active;
      toggle.textContent = active ? "Show static chart" : "Show interactive chart";
    });
    const start = () => {
      if (state.failed || state.handle || figure.closest("details:not([open])") ||
          figure.getBoundingClientRect().width === 0 || !fallback.getClientRects().length) return;
      try { mount(state); } catch { state.failed = true; restore(state); }
    };
    const resize = new ResizeObserver(start);
    resize.observe(figure);
    state.observer = resize;
    state.start = start;
    start();
  }

  function themeChanged() {
    for (const state of states) {
      if (!state.handle || !state.figure.classList.contains("chart-enhanced")) continue;
      restore(state);
      try { mount(state); } catch { state.failed = true; restore(state); }
    }
  }
  theme.addEventListener?.("change", themeChanged);
  document.querySelectorAll("figure[data-chart]").forEach(figure => {
    try { enhance(figure); } catch { /* Static SVG and numeric tables remain visible. */ }
  });
  document.addEventListener("toggle", event => {
    for (const state of states) {
      if (event.target.contains?.(state.figure)) state.start();
    }
  }, true);
  window.addEventListener("pagehide", () => {
    for (const state of states) {
      state.observer.disconnect();
      try { restore(state); } catch { /* Static SVG remains available. */ }
    }
  });
  window.addEventListener("pageshow", event => {
    if (!event.persisted) return;
    for (const state of states) {
      state.observer.observe(state.figure);
      state.start();
    }
  });
})();
