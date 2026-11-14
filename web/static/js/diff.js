// Renders an item's diff slices, and the "Show whole declaration" toggle
// for modified or added symbol items.

import { h } from "./dom.js";
import { getSource } from "./api.js";
import { update } from "./store.js";

// declarationCache holds, per item ID, the fetched whole-declaration text:
// {loading: true} | {error} | {start, lines, addNewNos}.
const declarationCache = new Map();
// expanded holds the item IDs whose declaration is currently shown.
const expanded = new Set();

// render returns the diff panel for item.
export function render(item) {
  const container = h("div", { class: "diff" });

  if (item.binary) {
    container.appendChild(h("div", { class: "diff-empty" }, "Binary file"));
    return container;
  }
  if (!item.slices || item.slices.length === 0) {
    container.appendChild(h("div", { class: "diff-empty" }, "No content changes"));
    return container;
  }

  renderSlices(container, item.slices);

  if (item.kind === "symbol" && (item.status === "modified" || item.status === "added")) {
    container.appendChild(renderDeclarationToggle(item));
  }
  return container;
}

function renderSlices(container, slices) {
  let prevPath = null;
  for (const slice of slices) {
    if (slice.path !== prevPath) {
      container.appendChild(h("div", { class: "diff-file" }, slice.path));
    } else {
      container.appendChild(h("div", { class: "diff-sep" }, "⋯"));
    }
    container.appendChild(renderSliceTable(slice));
    prevPath = slice.path;
  }
}

function renderSliceTable(slice) {
  const rows = [];
  for (const line of slice.lines) {
    rows.push(renderLineRow(line));
    if (line.noEol) {
      rows.push(
        h(
          "tr",
          { class: "diff-noeol" },
          h("td", { class: "ln old" }),
          h("td", { class: "ln new" }),
          h("td", { class: "marker" }),
          h("td", { class: "code" }, "\\ No newline at end of file")
        )
      );
    }
  }
  return h("table", { class: "diff-table" }, h("tbody", null, rows));
}

function renderLineRow(line) {
  const cls = line.kind === "add" ? "add" : line.kind === "del" ? "del" : "context";
  const marker = line.kind === "add" ? "+" : line.kind === "del" ? "-" : " ";
  return h(
    "tr",
    { class: cls },
    h("td", { class: "ln old" }, line.oldNo || ""),
    h("td", { class: "ln new" }, line.newNo || ""),
    h("td", { class: "marker" }, marker),
    h("td", { class: "code" }, line.text)
  );
}

function renderDeclarationToggle(item) {
  const isOpen = expanded.has(item.id);
  const wrap = h("div", { class: "declaration" });
  wrap.appendChild(
    h(
      "button",
      { type: "button", class: "link-btn", onclick: () => toggleDeclaration(item) },
      isOpen ? "Hide whole declaration" : "Show whole declaration"
    )
  );
  if (isOpen) {
    renderDeclarationBody(item, wrap);
  }
  return wrap;
}

function toggleDeclaration(item) {
  if (expanded.has(item.id)) {
    expanded.delete(item.id);
    update(() => {});
    return;
  }
  expanded.add(item.id);
  if (declarationCache.has(item.id)) {
    update(() => {});
    return;
  }
  declarationCache.set(item.id, { loading: true });
  update(() => {});

  const sym = item.symbol;
  getSource("head", sym.file, sym.startLine, sym.endLine)
    .then((res) => {
      declarationCache.set(item.id, buildDeclaration(item, res));
      update(() => {});
    })
    .catch((err) => {
      declarationCache.set(item.id, { error: err.message });
      update(() => {});
    });
}

// buildDeclaration records which absolute line numbers of the fetched range
// appear as an "add" line in any of the item's slices.
function buildDeclaration(item, res) {
  const addNewNos = new Set();
  for (const slice of item.slices) {
    for (const line of slice.lines) {
      if (line.kind === "add") {
        addNewNos.add(line.newNo);
      }
    }
  }
  return { start: res.start, lines: res.lines, addNewNos };
}

function renderDeclarationBody(item, wrap) {
  const data = declarationCache.get(item.id);
  if (!data) {
    return;
  }
  if (data.loading) {
    wrap.appendChild(h("div", { class: "muted" }, "Loading…"));
    return;
  }
  if (data.error) {
    wrap.appendChild(h("div", { class: "error" }, data.error));
    return;
  }
  const rows = data.lines.map((text, i) => {
    const lineNo = data.start + i;
    const isAdd = data.addNewNos.has(lineNo);
    return h(
      "tr",
      { class: isAdd ? "add" : "context" },
      h("td", { class: "ln new" }, lineNo),
      h("td", { class: "marker" }, isAdd ? "+" : " "),
      h("td", { class: "code" }, text)
    );
  });
  wrap.appendChild(h("table", { class: "diff-table declaration-table" }, h("tbody", null, rows)));
}
