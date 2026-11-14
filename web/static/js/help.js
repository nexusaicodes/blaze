// The help overlay: shortcuts, what Blaze cannot see, and plan warnings.
// Closed with Esc or a click outside the panel.

import { h, clear } from "./dom.js";
import { store } from "./store.js";

const overlay = document.getElementById("overlay");

overlay.addEventListener("click", (event) => {
  if (event.target === overlay) {
    close();
  }
});

const SHORTCUTS = [
  ["j / ↓", "next item"],
  ["k / ↑", "previous item"],
  ["]", "first item of the next thread"],
  ["[", "first item of the previous thread"],
  ["r", "toggle reviewed on the selected item"],
  ["u", "jump to the next unreviewed item"],
  ["b", "show or hide the blast panel"],
  ["n", "focus the note box"],
  ["f", "switch between Plan and Files view"],
  ["?", "open this help"],
  ["Esc", "close this help, or leave the note box"],
];

// LIMITS is the "Known limits" table of 04-dodge.md, in prose form.
const LIMITS = [
  "Files excluded by build constraints for the current OS/arch are not loaded; the plan falls back to syntax-only symbols without blast data.",
  "Calls through function values, reflection or go:linkname are not seen.",
  "Generic types get no implements refs.",
  "Methods promoted through embedding get no method-level implements refs.",
  "Types declared in _test.go files: implements refs against non-test interfaces may be missed.",
  "Code that does not type-check yields partial refs; a warning is shown.",
  "cgo: only the Go side is analysed.",
];

// isOpen reports whether the overlay is currently shown.
export function isOpen() {
  return !overlay.hidden;
}

// open renders the overlay content and shows it.
export function open() {
  render();
  overlay.hidden = false;
}

// close hides the overlay.
export function close() {
  overlay.hidden = true;
}

function render() {
  clear(overlay);
  overlay.appendChild(
    h(
      "div",
      { class: "overlay-panel" },
      h("h2", null, "Keyboard shortcuts"),
      h(
        "dl",
        { class: "shortcut-list" },
        SHORTCUTS.flatMap(([key, desc]) => [h("dt", null, key), h("dd", null, desc)])
      ),
      h("h2", null, "What Blaze cannot see"),
      h("ul", null, LIMITS.map((text) => h("li", null, text))),
      renderWarnings()
    )
  );
}

function renderWarnings() {
  const warnings = (store.plan && store.plan.warnings) || [];
  if (warnings.length === 0) {
    return null;
  }
  return h("div", null, h("h2", null, "Plan warnings"), h("ul", null, warnings.map((w) => h("li", null, w))));
}
