// Renders the thread and item list in the sidebar.

import { h, clear } from "./dom.js";
import { store, update, isReviewed, threadProgress } from "./store.js";

const RISK_BADGE = { high: "HIGH", medium: "MED", low: "LOW" };

let lastSelectedId = null;

// render rebuilds container from scratch, preserving scroll position unless
// the selected item changed, in which case the selected row is scrolled
// into view instead.
export function render(container) {
  const savedScroll = container.scrollTop;
  clear(container);
  for (const thread of store.plan.threads) {
    container.appendChild(renderThread(thread));
  }

  const selectionChanged = lastSelectedId !== store.selectedId;
  lastSelectedId = store.selectedId;
  if (selectionChanged && store.selectedId !== null) {
    const row = container.querySelector(`[data-id="${store.selectedId}"]`);
    if (row) {
      row.scrollIntoView({ block: "nearest" });
      return;
    }
  }
  container.scrollTop = savedScroll;
}

function renderThread(thread) {
  const collapsed = store.collapsed.has(thread.id);
  const { done, total } = threadProgress(thread);

  const headerChildren = [
    h("span", { class: "collapse-arrow" }, collapsed ? "▶" : "▼"),
    h("span", { class: "thread-title" }, thread.title),
  ];
  const badge = RISK_BADGE[thread.risk.level];
  if (badge) {
    headerChildren.push(h("span", { class: `badge risk-${thread.risk.level}` }, badge));
  }
  headerChildren.push(h("span", { class: "thread-subtitle" }, thread.subtitle));
  headerChildren.push(h("span", { class: "thread-progress" }, `${done}/${total}`));

  const header = h("div", { class: "thread-header", onclick: () => toggleCollapse(thread.id) }, headerChildren);
  const wrap = h("div", { class: "thread" }, header);

  if (!collapsed) {
    for (const id of thread.itemIds) {
      wrap.appendChild(renderItemRow(store.plan.items[id]));
    }
  }
  return wrap;
}

function renderItemRow(item) {
  const selected = item.id === store.selectedId;
  const glyph = isReviewed(item) ? "✓" : selected ? "●" : "○";
  const classes = ["item-row"];
  if (selected) {
    classes.push("selected");
  }
  if (item.status === "added") {
    classes.push("added");
  }
  if (item.status === "deleted") {
    classes.push("deleted");
  }
  return h(
    "div",
    { class: classes.join(" "), dataset: { id: item.id }, onclick: () => selectItem(item.id) },
    h("span", { class: "glyph" }, glyph),
    h("span", { class: "row-title" }, item.title),
    h("span", { class: "tag role" }, item.role)
  );
}

function toggleCollapse(id) {
  update((s) => {
    if (s.collapsed.has(id)) {
      s.collapsed.delete(id);
    } else {
      s.collapsed.add(id);
    }
  });
}

function selectItem(id) {
  update((s) => {
    s.selectedId = id;
  });
}
