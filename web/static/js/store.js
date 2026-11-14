// Client-side state and derived data. One module-level object, no
// reactivity library: components read `store` directly and call `update`
// to mutate it and notify subscribers.

export const store = {
  plan: null, // from /api/plan
  state: null, // from /api/state
  order: [], // item IDs in review order (threads flattened)
  selectedId: null,
  view: "plan", // "plan" | "files"
  blastOpen: true,
  collapsed: new Set(), // thread IDs collapsed in the sidebar
};

const subscribers = [];

// isReviewed reports whether item has a mark in store.state.reviewed.
export function isReviewed(item) {
  return Object.prototype.hasOwnProperty.call(store.state.reviewed, item.contentHash);
}

// threadProgress returns {done, total} over a thread's items.
export function threadProgress(thread) {
  let done = 0;
  for (const id of thread.itemIds) {
    if (isReviewed(store.plan.items[id])) {
      done++;
    }
  }
  return { done, total: thread.itemIds.length };
}

// totalProgress returns {done, total} over every item in review order.
export function totalProgress() {
  let done = 0;
  for (const id of store.order) {
    if (isReviewed(store.plan.items[id])) {
      done++;
    }
  }
  return { done, total: store.order.length };
}

// nextItem returns the neighbour of id in store.order, delta steps away,
// clamped to the ends of the list.
export function nextItem(id, delta) {
  if (store.order.length === 0) {
    return null;
  }
  const idx = store.order.indexOf(id);
  const base = idx === -1 ? 0 : idx;
  let next = base + delta;
  if (next < 0) {
    next = 0;
  }
  if (next > store.order.length - 1) {
    next = store.order.length - 1;
  }
  return store.order[next];
}

// nextUnreviewed returns the next unreviewed item after id in store.order,
// wrapping around; null if every item is reviewed (or there are none).
export function nextUnreviewed(id) {
  if (store.order.length === 0) {
    return null;
  }
  const start = id === null ? -1 : store.order.indexOf(id);
  for (let step = 1; step <= store.order.length; step++) {
    const idx = (start + step + store.order.length) % store.order.length;
    const candidate = store.order[idx];
    if (!isReviewed(store.plan.items[candidate])) {
      return candidate;
    }
  }
  return null;
}

// subscribe registers fn to be called after every update.
export function subscribe(fn) {
  subscribers.push(fn);
}

// update applies mutator to store, then notifies every subscriber.
export function update(mutator) {
  mutator(store);
  for (const fn of subscribers) {
    fn();
  }
}
