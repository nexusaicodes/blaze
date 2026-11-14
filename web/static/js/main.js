// Start-up, routing and top-level render.

import { h, clear } from "./dom.js";
import { getPlan, getState } from "./api.js";
import { store, update, subscribe, totalProgress, nextUnreviewed } from "./store.js";
import * as keys from "./keys.js";
import * as sidebar from "./sidebar.js";
import * as item from "./item.js";
import * as files from "./files.js";
import * as help from "./help.js";

const topbarEl = document.getElementById("topbar");
const sidebarEl = document.getElementById("sidebar");
const mainEl = document.getElementById("main");

async function start() {
  try {
    const [plan, state] = await Promise.all([getPlan(), getState()]);
    store.plan = plan;
    store.state = state;
    store.order = buildOrder(plan);
    for (const thread of plan.threads) {
      if (thread.kind === "mechanical" || thread.kind === "generated") {
        store.collapsed.add(thread.id);
      }
    }
    applyRoute();
    subscribe(render);
    render();
    keys.init();
    window.addEventListener("hashchange", () => {
      applyRoute();
      render();
    });
  } catch (err) {
    clear(mainEl);
    mainEl.textContent = err && err.message ? err.message : String(err);
  }
}

function buildOrder(plan) {
  const ids = [];
  for (const thread of plan.threads) {
    ids.push(...thread.itemIds);
  }
  return ids;
}

// applyRoute selects the item named by the URL fragment, or falls back to
// the next unreviewed item (or the first item) when there is none.
function applyRoute() {
  const match = /^#\/item\/(.+)$/.exec(location.hash);
  if (match && store.plan.items[match[1]]) {
    store.selectedId = match[1];
    return;
  }
  const fallback = nextUnreviewed(null);
  store.selectedId = fallback !== null ? fallback : store.order[0] ?? null;
}

// syncHash mirrors store.selectedId into the URL fragment.
function syncHash() {
  const want = store.selectedId ? `#/item/${store.selectedId}` : "";
  if (location.hash !== want) {
    history.replaceState(null, "", want === "" ? location.pathname + location.search : want);
  }
}

function render() {
  syncHash();
  renderTopbar(topbarEl);
  sidebar.render(sidebarEl);
  if (store.view === "plan") {
    item.render(mainEl);
  } else {
    files.render(mainEl);
  }
  const { done, total } = totalProgress();
  document.title = `${done}/${total} · ${store.plan.target.title} · Blaze`;
}

function renderTopbar(container) {
  clear(container);
  const target = store.plan.target;

  let left;
  if (target.kind === "github") {
    left = h(
      "a",
      { href: target.url, target: "_blank", rel: "noopener noreferrer" },
      `#${target.number} ${target.title}`
    );
  } else {
    left = h("span", { class: "topbar-title" }, target.title);
  }

  const middle = h(
    "span",
    { class: "topbar-range" },
    `base ${target.baseSha.slice(0, 7)} → head ${target.headSha.slice(0, 7)}`
  );

  const { done, total } = totalProgress();
  const right = h(
    "div",
    { class: "topbar-right" },
    h("span", { class: "progress-text" }, `${done}/${total} reviewed`),
    renderProgressBar(done, total),
    h("button", { type: "button", class: "icon-btn", onclick: () => help.open() }, "?")
  );
  const warnings = store.plan.warnings || [];
  if (warnings.length > 0) {
    right.appendChild(
      h(
        "button",
        { type: "button", class: "warn-btn", onclick: () => help.open() },
        `⚠ ${warnings.length} warning${warnings.length === 1 ? "" : "s"}`
      )
    );
  }

  container.appendChild(h("div", { class: "topbar-row1" }, left, middle, right));
  container.appendChild(
    h("div", { class: "topbar-row2" }, renderTab("Plan", "plan"), renderTab("Files", "files"))
  );
}

function renderProgressBar(done, total) {
  const fill = h("div", { class: "progress-fill" });
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  fill.style.setProperty("width", `${pct}%`);
  return h("div", { class: "progress-bar" }, fill);
}

function renderTab(label, view) {
  const active = store.view === view;
  return h(
    "button",
    {
      type: "button",
      class: active ? "tab active" : "tab",
      onclick: () =>
        update((s) => {
          s.view = view;
        }),
    },
    label
  );
}

start();
