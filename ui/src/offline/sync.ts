import { useSyncExternalStore } from "react";

import type { Me } from "@/pages/Boards/loaders";
import { apiFetch } from "@/utils/apiFetch";
import { newChangeId } from "@/utils/socket";

import { alreadySaved, toRequest, touchedIds, type Change } from "./changes";
import {
  addPending,
  applyChange,
  loadPending,
  loadRejected,
  nextPending,
  reject,
  removePending,
} from "./copy";

// Offline means the browser says so. A server that can't be reached while
// the browser is online keeps changes waiting all the same, and loaders
// read the Offline copy then too.
export const isOffline = () => !navigator.onLine;

type SyncState = {
  offline: boolean;
  syncing: boolean;
  // Boards, cards and tasks with changes the server hasn't accepted yet.
  pendingIds: Set<string>;
  pendingCount: number;
  rejectedCount: number;
};

let state: SyncState = {
  offline: isOffline(),
  syncing: false,
  pendingIds: new Set(),
  pendingCount: 0,
  rejectedCount: 0,
};
const listeners = new Set<() => void>();

function setState(update: Partial<SyncState>) {
  state = { ...state, ...update };
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useSync() {
  return useSyncExternalStore(subscribe, () => state);
}

let me: Me | null = null;
let onRefused: (count: number) => void = () => {};

// Starts tracking one person's pending changes, and sends them; called
// once who is signed in is known (from the server, or from the offline
// copy). Doesn't wait for them to be sent: a page that needs them on the
// server first waits for sync() itself.
export async function startSync(user: Me) {
  me = user;
  await refreshCounts();
  if (!isOffline()) void sync();
}

export function stopSync() {
  me = null;
  clearTimeout(retry);
  setState({ pendingIds: new Set(), pendingCount: 0, rejectedCount: 0 });
}

// What to do once the server has refused changes, so what's on screen no
// longer matches it: say so and load the page's data again.
export function setOnRefused(callback: (count: number) => void) {
  onRefused = callback;
}

export function refused(count = 1) {
  onRefused(count);
}

// Sends a change over HTTP, under an id the server's broadcast of it will
// carry, so this app can tell the broadcast is its own.
function post(change: Change) {
  const { url, init } = toRequest(change);
  return apiFetch(url, {
    ...init,
    headers: { ...init.headers, "X-Change-Id": newChangeId() },
  });
}

async function refreshCounts() {
  if (!me) return;
  const [pending, rejected] = await Promise.all([
    loadPending(me.id),
    loadRejected(me.id),
  ]);
  setState({
    pendingIds: new Set(pending.flatMap(({ change }) => touchedIds(change))),
    pendingCount: pending.length,
    rejectedCount: rejected.length,
  });
}

// Makes a change: sends it now when online, or keeps it as a Pending change
// when offline, when the server can't be reached or answers with a server
// error, or while earlier ones are still waiting, so they reach the server
// in order. Resolves false only when the server refuses it.
export function submit(change: Change): Promise<boolean> {
  // One at a time, so a change made before the last one's answer came back
  // still reaches the server after it.
  const result = submitting.then(() => submitNow(change));
  submitting = result.catch(() => {});
  return result;
}

let submitting: Promise<unknown> = Promise.resolve();

// Resolves once the changes made so far have each reached the server or
// been kept as Pending changes, and sending the Pending ones has had its go.
export async function settled() {
  await submitting;
  await running;
}

async function submitNow(change: Change) {
  if (me && (isOffline() || state.pendingCount > 0)) {
    await keep(me, change);
    void sync();
    return true;
  }

  let res: Response | null;
  try {
    res = await post(change);
  } catch {
    res = null;
  }

  if (res && (res.ok || alreadySaved(change, res.status))) {
    if (me) await applyChange(me, change);
    return true;
  }
  if (me && (!res || res.status >= 500)) {
    await keep(me, change);
    retryLater();
    return true;
  }
  return false;
}

async function keep(user: Me, change: Change) {
  await addPending(user.id, change);
  await applyChange(user, change);
  await refreshCounts();
}

let running: Promise<void> | null = null;

// After a network failure or a server error, sync tries again on its own,
// waiting longer each time it fails in a row.
const FIRST_RETRY = 5_000;
const LAST_RETRY = 5 * 60_000;
let retryDelay = FIRST_RETRY;
let retry: ReturnType<typeof setTimeout> | undefined;

function retryLater() {
  clearTimeout(retry);
  retry = setTimeout(() => void sync(), retryDelay);
  retryDelay = Math.min(retryDelay * 2, LAST_RETRY);
}

// Sends Pending changes in the order they were made. One the server
// refuses becomes a Rejected change, kept for review; a network failure,
// a server error or an expired session stops here, to try again later.
export function sync(): Promise<void> {
  running ??= send().finally(() => (running = null));
  return running;
}

async function send() {
  clearTimeout(retry);
  if (!me || isOffline()) return;
  const user = me;
  let refusedCount = 0;
  let failed = false;
  setState({ syncing: true });

  try {
    // Takes the oldest change each time round, so ones made while this
    // runs are sent too, after the ones before them.
    while (!isOffline() && me === user) {
      const pending = await nextPending(user.id);
      if (!pending) break;

      let res: Response;
      try {
        res = await post(pending.change);
      } catch {
        failed = true;
        break;
      }

      if (res.ok || alreadySaved(pending.change, res.status)) {
        await removePending(user.id, pending.seq);
      } else if (res.status >= 500) {
        failed = true;
        break;
      } else if (res.status === 401) {
        // Signed out elsewhere: the next page load goes to sign-in.
        break;
      } else {
        await reject(user.id, pending, res.status, await res.text());
        refusedCount++;
      }
      await refreshCounts();
    }
  } finally {
    setState({ syncing: false });
  }

  if (me !== user) return;
  if (failed) retryLater();
  else retryDelay = FIRST_RETRY;
  if (refusedCount > 0) refused(refusedCount);
}

window.addEventListener("online", () => {
  setState({ offline: false });
  void sync();
});
window.addEventListener("offline", () => setState({ offline: true }));
