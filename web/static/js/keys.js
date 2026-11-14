// One keydown listener on document for the shortcuts of 07-web-ui.md.

import { store, update, nextItem, nextUnreviewed } from "./store.js";
import { setReviewed } from "./api.js";
import * as help from "./help.js";

// init installs the key handler. Called once at start-up.
export function init() {
  document.addEventListener("keydown", onKeyDown);
}

function onKeyDown(event) {
  if (event.ctrlKey || event.altKey || event.metaKey) {
    return;
  }

  const target = event.target;
  const inField = target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement;
  if (inField) {
    if (event.key === "Escape") {
      target.blur();
    }
    return;
  }

  switch (event.key) {
    case "j":
    case "ArrowDown":
      selectByDelta(1);
      break;
    case "k":
    case "ArrowUp":
      selectByDelta(-1);
      break;
    case "]":
      selectThreadByDelta(1);
      break;
    case "[":
      selectThreadByDelta(-1);
      break;
    case "r":
      toggleReviewed();
      break;
    case "u":
      jumpUnreviewed();
      break;
    case "b":
      update((s) => {
        s.blastOpen = !s.blastOpen;
      });
      break;
    case "n":
      focusNote();
      break;
    case "f":
      update((s) => {
        s.view = s.view === "plan" ? "files" : "plan";
      });
      break;
    case "?":
      help.open();
      break;
    case "Escape":
      if (help.isOpen()) {
        help.close();
      }
      break;
    default:
      return;
  }
  event.preventDefault();
}

function selectByDelta(delta) {
  const id = nextItem(store.selectedId, delta);
  if (id !== null) {
    update((s) => {
      s.selectedId = id;
    });
  }
}

function selectThreadByDelta(delta) {
  if (!store.plan) {
    return;
  }
  const threads = store.plan.threads;
  if (threads.length === 0) {
    return;
  }
  const currentIdx = threads.findIndex((t) => t.itemIds.includes(store.selectedId));
  let idx = currentIdx === -1 ? 0 : currentIdx + delta;
  if (idx < 0) {
    idx = 0;
  }
  if (idx > threads.length - 1) {
    idx = threads.length - 1;
  }
  const thread = threads[idx];
  if (thread.itemIds.length > 0) {
    update((s) => {
      s.selectedId = thread.itemIds[0];
    });
  }
}

function toggleReviewed() {
  const item = store.plan.items[store.selectedId];
  if (!item) {
    return;
  }
  const marking = !Object.prototype.hasOwnProperty.call(store.state.reviewed, item.contentHash);
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

function jumpUnreviewed() {
  const id = nextUnreviewed(store.selectedId);
  if (id !== null) {
    update((s) => {
      s.selectedId = id;
    });
  }
}

function focusNote() {
  const box = document.getElementById("note-box");
  if (box) {
    box.focus();
  }
}
