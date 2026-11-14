// Fetch wrappers for the /api/ routes. Every request carries the token
// read once from the page's meta tag.

const TOKEN = document.querySelector('meta[name="blaze-token"]').content;

// request performs one fetch against the API and returns the decoded JSON
// body. A non-2xx response throws new Error(body.error || statusText).
async function request(method, url, body) {
  const headers = { "X-Blaze-Token": TOKEN };
  const init = { method, headers };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  const response = await fetch(url, init);
  const data = await response.json();
  if (!response.ok) {
    throw new Error((data && data.error) || response.statusText);
  }
  return data;
}

export async function getPlan() {
  return request("GET", "/api/plan");
}

export async function getState() {
  return request("GET", "/api/state");
}

export async function setReviewed(id, reviewed) {
  return request("PUT", `/api/items/${encodeURIComponent(id)}/review`, { reviewed });
}

export async function setNote(id, note) {
  return request("PUT", `/api/items/${encodeURIComponent(id)}/note`, { note });
}

export async function getSource(side, path, start, end) {
  const params = new URLSearchParams({
    side,
    path,
    start: String(start),
    end: String(end),
  });
  return request("GET", `/api/source?${params.toString()}`);
}
