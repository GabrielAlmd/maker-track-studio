// Test-only HTTP fixtures. Paginated list requests return an envelope; lookup
// requests without parameters still use the legacy array contract.
export function apiPage(items = [], { total = items.length, page = 1, pageSize = 25 } = {}) {
  return { items, total, page, page_size: pageSize };
}

export function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}
