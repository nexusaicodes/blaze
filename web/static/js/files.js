// The Files tab: a flat table of every changed file, the escape hatch from
// the thread view.

import { h, clear } from "./dom.js";
import { store, update } from "./store.js";

export function render(container) {
  clear(container);
  const tbody = h("tbody");
  for (const file of store.plan.files) {
    tbody.appendChild(
      h(
        "tr",
        { class: "file-row", onclick: () => selectFile(file) },
        h("td", { class: "file-status" }, file.status),
        h("td", { class: "file-path" }, file.path),
        h("td", { class: "file-added" }, `+${file.added}`),
        h("td", { class: "file-deleted" }, `-${file.deleted}`)
      )
    );
  }
  container.appendChild(h("table", { class: "files-table" }, tbody));
}

function selectFile(file) {
  update((s) => {
    s.view = "plan";
    s.selectedId = file.itemIds.length > 0 ? file.itemIds[0] : null;
  });
}
