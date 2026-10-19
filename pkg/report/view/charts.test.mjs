import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const script = readFileSync(new URL("./charts.js", import.meta.url), "utf8");

class Element {
  constructor() {
    this.children = [];
    this.hidden = false;
    this.style = {};
    this.attributes = {};
    this.listeners = {};
    this.classes = new Set();
    this.classList = {
      add: name => this.classes.add(name),
      remove: name => this.classes.delete(name),
      contains: name => this.classes.has(name),
      toggle: name => {
        if (this.classes.delete(name)) return false;
        this.classes.add(name);
        return true;
      }
    };
  }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  setAttribute(name, value) { this.attributes[name] = value; }
  addEventListener(name, callback) { this.listeners[name] = callback; }
  getBoundingClientRect() { return { width: 800 }; }
  getClientRects() { return [{}]; }
  closest() { return this.closed ? this : null; }
  querySelector(selector) {
    if (selector === "h3") return { textContent: "Recorded metric" };
    if (selector === "canvas") return this.children.length ? {} : null;
    return this.fallback;
  }
}

function setup(source, behavior = "ready", closed = false) {
  const figure = new Element();
  figure.closed = closed;
  figure.dataset = { chart: JSON.stringify(source) };
  figure.fallback = new Element();
  figure.fallback.before = (target, toggle) => { figure.target = target; figure.toggle = toggle; };
  const state = { calls: [], destroyed: 0, observers: [], events: {} };
  const timers = [];
  class Observer {
    constructor(callback) { this.callback = callback; state.observers.push(this); }
    observe() { this.disconnected = false; }
    disconnect() { this.disconnected = true; }
  }
  const chart = {
    mount(target, series, options) {
      state.calls.push({ series, options });
      if (behavior === "throw") throw new Error("mount failure");
      const config = { scales: {}, axes: [{}, {}], series: [{}, ...series.map(() => ({}))] };
      const Plot = { paths: { linear: () => "linear", bars: () => "bars" } };
      options.configure(config, Plot);
      state.calls.at(-1).config = config;
      const plot = { destroy() { state.destroyed++; } };
      options.mounted(plot);
      target.append(new Element());
      if (behavior === "ready") config.hooks.ready[0](plot);
      return { destroy() { state.destroyed++; target.replaceChildren(); } };
    }
  };
  const window = {
    StroppyChart: chart, ResizeObserver: Observer,
    matchMedia: () => ({ addEventListener(name, callback) { state.themeChanged = callback; } }),
    addEventListener(name, callback) { state.events[name] = callback; }
  };
  vm.runInNewContext(script, {
    window, StroppyChart: chart, ResizeObserver: Observer,
    document: { createElement: () => new Element(), querySelectorAll: () => [figure], addEventListener() {} },
    getComputedStyle: () => ({ getPropertyValue: () => "#5b8ef0" }), devicePixelRatio: 1,
    setTimeout: callback => { timers.push(callback); return callback; }, clearTimeout() {}, Intl
  });
  return { figure, state, timers };
}

const timeline = {
  kind: "timeline", unit: "ms", series: [
    { title: "A", color: 1, interval: 1, points: [
      { time: "0", value: "100", sequence: 1 },
      { time: "1", value: "110", sequence: 2 },
      { time: "4", value: "120", sequence: 5 }
    ] },
    { title: "B", color: 2, pattern: "8 4", interval: 0.5, points: [
      { time: "0.5", value: "102", sequence: 1 },
      { time: "1.5", value: "112", sequence: 3 }
    ] }
  ]
};
const settled = () => new Promise(resolve => setImmediate(resolve));

test("elapsed samples align without invented observations or bridged gaps", async () => {
  const { figure, state } = setup(timeline);
  await settled();
  const { data } = state.calls[0].options;
  assert.deepEqual(Array.from(data[0]), [0, 0.5, 1, 1.5, 2.5, 4]);
  assert.equal(data[1][1], undefined);
  assert.equal(data[1][4], null);
  assert.equal(data[2][2], null);
  assert.equal(state.calls[0].config.scales.x.time, false);
  assert.equal(state.calls[0].config.series[1].spanGaps, false);
  assert.equal(state.calls[0].config.series[1].paths, "linear");
  assert.equal(figure.classList.contains("chart-enhanced"), true);
  figure.toggle.listeners.click();
  assert.equal(figure.classList.contains("chart-enhanced"), false);
  assert.equal(figure.target.hidden, true);
});

test("mount failure and timeout retain static view", async () => {
  for (const behavior of ["throw", "pending"]) {
    const { figure, state, timers } = setup(timeline, behavior);
    for (const timer of timers) timer();
    await settled();
    assert.equal(figure.classList.contains("chart-enhanced"), false);
    assert.equal(figure.target.hidden, true);
    assert.equal(figure.toggle.hidden, true);
    if (behavior === "pending") assert.equal(state.destroyed, 1);
  }
});

test("closed disclosures defer chart creation; theme changes redraw active charts", async () => {
  const { figure, state } = setup(timeline, "ready", true);
  assert.equal(state.calls.length, 0);
  figure.closed = false;
  state.observers[0].callback();
  await settled();
  assert.equal(state.calls.length, 1);
  state.themeChanged();
  await settled();
  assert.equal(state.calls.length, 2);
  assert.equal(state.destroyed, 1);
  assert.equal(figure.classList.contains("chart-enhanced"), true);
  state.events.pagehide();
  assert.equal(state.observers[0].disconnected, true);
  assert.equal(state.destroyed, 2);
  assert.equal(figure.classList.contains("chart-enhanced"), false);
  assert.equal(figure.target.hidden, true);
  state.events.pageshow({ persisted: false });
  assert.equal(state.calls.length, 2);
  state.events.pageshow({ persisted: true });
  await settled();
  assert.equal(state.calls.length, 3);
  assert.equal(state.observers[0].disconnected, false);
  assert.equal(figure.classList.contains("chart-enhanced"), true);
  assert.equal(figure.target.hidden, false);
});

test("downsampled and invalid clocks never become continuous curves", async () => {
  const sampled = structuredClone(timeline);
  sampled.series[0].omitted = 10;
  const { state } = setup(sampled);
  assert.equal(state.calls[0].options.data[1][1], null);
  const invalid = structuredClone(timeline);
  invalid.series[0].points[1].time = "0";
  const { figure } = setup(invalid);
  await settled();
  assert.equal(figure.classList.contains("chart-enhanced"), false);
});
