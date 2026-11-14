// Renders the blast radius panel: callers of the selected item's symbol,
// grouped by section and package, with on-demand source context.

import { h } from "./dom.js";
import { getSource } from "./api.js";
import { store, update } from "./store.js";

// contextCache holds, per "file:line" key, the fetched context:
// {loading: true} | {error} | {start, lines, highlight}.
const contextCache = new Map();
// openKeys holds the "file:line" keys currently expanded.
const openKeys = new Set();

// render returns the blast panel for item. item.blast must not be null.
export function render(item) {
  const blast = item.blast;
  const container = h("div", { class: "blast", hidden: !store.blastOpen });
  container.appendChild(h("h3", null, "Blast radius"));
  container.appendChild(h("div", { class: "blast-summary" }, summaryText(blast)));

  const outside = [];
  const inPr = [];
  const tests = [];
  for (const caller of blast.callers) {
    if (caller.isTest) {
      tests.push(caller);
    } else if (caller.inPr) {
      inPr.push(caller);
    } else {
      outside.push(caller);
    }
  }

  if (outside.length > 0) {
    container.appendChild(renderSection("Outside this PR", outside));
  } else if (blast.outsideCount === 0 && item.status === "modified") {
    container.appendChild(h("div", { class: "muted" }, "No callers outside this PR."));
  }
  if (inPr.length > 0) {
    container.appendChild(renderSection("In this PR", inPr));
  }
  if (tests.length > 0) {
    container.appendChild(renderSection("Tests", tests));
  }
  if (blast.truncated) {
    container.appendChild(h("div", { class: "muted" }, "Showing the first 200 callers."));
  }
  return container;
}

function summaryText(blast) {
  const capped = blast.transitiveCapped ? "+" : "";
  return (
    `${blast.outsideCount} outside · ${blast.inPrCount} in PR · ${blast.testCount} tests` +
    ` · ${blast.transitiveCount} within 3 hops${capped}`
  );
}

function renderSection(title, callers) {
  const section = h("div", { class: "blast-section" });
  section.appendChild(h("h4", null, title));
  let pkg = null;
  for (const caller of callers) {
    if (caller.pkg !== pkg) {
      section.appendChild(h("div", { class: "blast-pkg" }, caller.pkg));
      pkg = caller.pkg;
    }
    section.appendChild(renderCaller(caller));
  }
  return section;
}

function renderCaller(caller) {
  const key = `${caller.file}:${caller.line}`;
  const parts = [
    h("span", { class: "caller-name" }, caller.name),
    h("span", { class: "caller-loc" }, `${caller.file}:${caller.line}`),
    h("span", { class: "caller-kind" }, caller.kind),
  ];
  if (caller.sites > 1) {
    parts.push(h("span", { class: "caller-sites" }, `×${caller.sites}`));
  }
  if (caller.via) {
    parts.push(h("span", { class: "caller-via" }, `via ${caller.via}`));
  }
  parts.push(h("code", { class: "caller-snippet" }, caller.snippet));

  const row = h("div", { class: "caller-row", onclick: () => onCallerClick(caller) }, parts);
  const wrap = h("div", { class: "caller" }, row);
  if (!caller.inPr && openKeys.has(key)) {
    wrap.appendChild(renderContext(key));
  }
  return wrap;
}

function onCallerClick(caller) {
  if (caller.inPr) {
    update((s) => {
      s.selectedId = caller.itemId;
    });
    return;
  }

  const key = `${caller.file}:${caller.line}`;
  if (openKeys.has(key)) {
    openKeys.delete(key);
    update(() => {});
    return;
  }
  openKeys.add(key);
  if (contextCache.has(key)) {
    update(() => {});
    return;
  }

  contextCache.set(key, { loading: true });
  update(() => {});

  const start = Math.max(1, caller.line - 5);
  const end = caller.line + 5;
  getSource("head", caller.file, start, end)
    .then((res) => {
      contextCache.set(key, { start: res.start, lines: res.lines, highlight: caller.line });
      update(() => {});
    })
    .catch((err) => {
      contextCache.set(key, { error: err.message });
      update(() => {});
    });
}

function renderContext(key) {
  const data = contextCache.get(key);
  const wrap = h("div", { class: "caller-context" });
  if (!data || data.loading) {
    wrap.appendChild(h("div", { class: "muted" }, "Loading…"));
    return wrap;
  }
  if (data.error) {
    wrap.appendChild(h("div", { class: "error" }, data.error));
    return wrap;
  }
  const rows = data.lines.map((text, i) => {
    const lineNo = data.start + i;
    return h(
      "tr",
      { class: lineNo === data.highlight ? "highlight" : "context" },
      h("td", { class: "ln new" }, lineNo),
      h("td", { class: "code" }, text)
    );
  });
  wrap.appendChild(h("table", { class: "diff-table context-table" }, h("tbody", null, rows)));
  return wrap;
}
