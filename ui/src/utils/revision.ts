const revisions = new WeakMap<object, number>();
let next = 0;

// A number that changes whenever a loader returns new data, for keying
// components that copy their loader data into state, so a reload of the
// data (such as after a sync) starts them afresh.
export function revisionOf(data: object) {
  let revision = revisions.get(data);
  if (revision === undefined) {
    revision = next++;
    revisions.set(data, revision);
  }
  return revision;
}
