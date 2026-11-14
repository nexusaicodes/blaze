// Tiny DOM-building helper. No framework, no innerHTML: every node is built
// with document.createElement and every string is set with textContent.

// h builds one element: h("div", {class: "row", onclick: fn, dataset: {id}},
// child1, "text", …). `class` sets className; keys starting with "on" add
// event listeners (the rest of the key, lower-cased, is the event name);
// `dataset` assigns into node.dataset; `hidden`, `disabled` and `checked`
// are set as properties; every other key is set with setAttribute. String
// and number children become text nodes. null, undefined and false children
// are skipped. Arrays are flattened.
export function h(tag, props, ...children) {
  const node = document.createElement(tag);
  if (props) {
    for (const key of Object.keys(props)) {
      const value = props[key];
      if (value === undefined) {
        continue;
      }
      if (key === "class") {
        node.className = value;
      } else if (key.length > 2 && key.startsWith("on")) {
        node.addEventListener(key.slice(2).toLowerCase(), value);
      } else if (key === "dataset") {
        Object.assign(node.dataset, value);
      } else if (key === "hidden" || key === "disabled" || key === "checked") {
        node[key] = value;
      } else {
        node.setAttribute(key, value);
      }
    }
  }
  appendChildren(node, children);
  return node;
}

function appendChildren(node, children) {
  for (const child of children) {
    if (child === null || child === undefined || child === false) {
      continue;
    }
    if (Array.isArray(child)) {
      appendChildren(node, child);
    } else if (child instanceof Node) {
      node.appendChild(child);
    } else {
      node.appendChild(document.createTextNode(String(child)));
    }
  }
}

// clear removes all children of node.
export function clear(node) {
  while (node.firstChild) {
    node.removeChild(node.firstChild);
  }
}
