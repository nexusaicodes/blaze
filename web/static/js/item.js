// Renders the item pane: header, meta, links, diff, blast and the note box.
//
// The note textarea is the one exception to "rebuild from scratch": while
// it has focus it is kept as the same live DOM node (never removed from the
// document), so typing is never interrupted by an unrelated store update.

import { h, clear } from "./dom.js";
import { store, update, isReviewed, nextItem, nextUnreviewed } from "./store.js";
import { setReviewed, setNote } from "./api.js";
import * as diff from "./diff.js";
import * as blast from "./blast.js";

const RISK_BADGE = { high: "HIGH", medium: "MED", low: "LOW" };

let noteTimer = null;
let liveNoteWrap = null;
let liveNoteItemId = null;

// render fills container with the pane for the selected item.
export function render(container) {
  const item = store.plan.items[store.selectedId];
  if (!item) {
    clear(container);
    liveNoteWrap = null;
    liveNoteItemId = null;
    container.appendChild(h("div", { class: "empty" }, "No item selected."));
    return;
  }

  const box = liveNoteWrap ? liveNoteWrap.querySelector("textarea") : null;
  const keepNote = liveNoteItemId === item.id && box !== null && document.activeElement === box;

  if (!keepNote) {
    clear(container);
    liveNoteWrap = buildNoteWrap(item);
    liveNoteItemId = item.id;
    for (const node of beforeNoteNodes(item)) {
      container.appendChild(node);
    }
    container.appendChild(liveNoteWrap);
    container.appendChild(renderActions(item));
    return;
  }

  for (const child of Array.from(container.children)) {
    if (child !== liveNoteWrap) {
      container.removeChild(child);
    }
  }
  for (const node of beforeNoteNodes(item)) {
    container.insertBefore(node, liveNoteWrap);
  }
  container.appendChild(renderActions(item));
}

// beforeNoteNodes builds everything that appears above the note box, fresh
// every time (only the note box itself is preserved across renders).
function beforeNoteNodes(item) {
  const nodes = [renderHeader(item), renderMeta(item)];
  if (item.roleReason) {
    nodes.push(h("div", { class: "why-here" }, h("strong", null, "Why here: "), item.roleReason));
  }
  const risk = renderRisk(item);
  if (risk) {
    nodes.push(risk);
  }
  const links = renderLinks(item);
  if (links) {
    nodes.push(links);
  }
  if (item.symbol && item.symbol.resolved === false) {
    nodes.push(
      h("div", { class: "notice" }, "No type information for this file. Blast radius is not available.")
    );
  }
  nodes.push(h("hr"));
  nodes.push(diff.render(item));
  if (item.blast) {
    nodes.push(h("hr"));
    nodes.push(blast.render(item));
  }
  nodes.push(h("hr"));
  return nodes;
}

function renderHeader(item) {
  const children = [h("span", { class: "item-title" }, item.title), h("span", { class: "tag role" }, item.role)];
  const badge = RISK_BADGE[item.risk.level];
  if (badge) {
    children.push(h("span", { class: `badge risk-${item.risk.level}` }, badge));
  }
  return h("div", { class: "item-header" }, children);
}

function renderMeta(item) {
  let text;
  if (item.kind === "symbol") {
    text = `${item.symbol.kind} · ${item.status} · ${item.path}:${item.symbol.declLine}`;
  } else {
    text = `${item.status} · ${item.path}`;
    if (item.oldPath) {
      text += ` · renamed from ${item.oldPath}`;
    }
  }
  const row = h("div", { class: "item-meta" }, text);
  if (item.signatureChanged) {
    row.appendChild(h("span", { class: "tag" }, "signature changed"));
  }
  return row;
}

function renderRisk(item) {
  if (item.risk.level === "none") {
    return null;
  }
  return h("div", { class: "item-risk" }, `Risk ${item.risk.score}: ${item.risk.reasons.join(" · ")}`);
}

function renderLinks(item) {
  const uses = renderLinkRow("Uses", item.uses);
  const usedBy = renderLinkRow("Used by", item.usedBy);
  if (!uses && !usedBy) {
    return null;
  }
  const row = h("div", { class: "item-links" });
  if (uses) {
    row.appendChild(uses);
  }
  if (usedBy) {
    row.appendChild(usedBy);
  }
  return row;
}

function renderLinkRow(label, links) {
  if (!links || links.length === 0) {
    return null;
  }
  const buttons = links.map((link) => {
    const target = store.plan.items[link.itemId];
    let text = target ? target.title : link.itemId;
    if (link.kind === "implements") {
      text += " (implements)";
    }
    if (link.member) {
      text += `.${link.member}`;
    }
    return h("button", { type: "button", class: "link-btn", onclick: () => selectItem(link.itemId) }, text);
  });
  return h("div", { class: "link-row" }, h("strong", null, `${label}: `), buttons);
}

function buildNoteWrap(item) {
  const initial = (store.state.notes && store.state.notes[item.key]) || "";
  const box = h("textarea", {
    id: "note-box",
    class: "note-box",
    placeholder: "Note…",
    oninput: () => scheduleSave(item),
    onblur: () => saveNote(item),
  });
  box.value = initial;
  return h("div", { class: "note-wrap" }, box, h("span", { class: "note-saved", id: "note-saved" }));
}

function scheduleSave(item) {
  if (noteTimer) {
    clearTimeout(noteTimer);
  }
  noteTimer = setTimeout(() => saveNote(item), 600);
}

function saveNote(item) {
  if (noteTimer) {
    clearTimeout(noteTimer);
    noteTimer = null;
  }
  if (!liveNoteWrap) {
    return;
  }
  const box = liveNoteWrap.querySelector("textarea");
  if (!box) {
    return;
  }
  setNote(item.id, box.value)
    .then((state) => {
      update((s) => {
        s.state = state;
      });
      const indicator = liveNoteWrap && liveNoteWrap.querySelector("#note-saved");
      if (indicator) {
        indicator.textContent = "saved";
        setTimeout(() => {
          indicator.textContent = "";
        }, 1500);
      }
    })
    .catch(() => {});
}

function renderActions(item) {
  const reviewed = isReviewed(item);
  const markBtn = h(
    "button",
    { type: "button", class: "btn", onclick: () => toggleReviewed(item) },
    reviewed ? "[r] Unmark" : "[r] Mark reviewed"
  );
  const prevBtn = h("button", { type: "button", class: "btn", onclick: () => selectDelta(-1) }, "[k] prev");
  const nextBtn = h("button", { type: "button", class: "btn", onclick: () => selectDelta(1) }, "[j] next");
  return h("div", { class: "item-actions" }, markBtn, prevBtn, nextBtn);
}

function toggleReviewed(item) {
  const marking = !isReviewed(item);
  setReviewed(item.id, marking)
    .then((state) => {
      update((s) => {
        s.state = state;
        if (marking) {
          const next = nextUnreviewed(item.id);
          if (next !== null) {
            s.selectedId = next;
          }
        }
      });
    })
    .catch(() => {});
}

function selectDelta(delta) {
  const id = nextItem(store.selectedId, delta);
  if (id !== null) {
    update((s) => {
      s.selectedId = id;
    });
  }
}

function selectItem(id) {
  update((s) => {
    s.selectedId = id;
  });
}
