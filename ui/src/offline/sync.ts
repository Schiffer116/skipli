import { useSyncExternalStore } from "react";

import type { Me } from "@/pages/Boards/loaders";
import { apiFetch } from "@/utils/apiFetch";

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

// Offline means the browser says so, nothing else: a server that can't be
// reached while the browser is online is an error, not offline mode.
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
let onRejected: () => void = () => {};

// Starts tracking one person's pending changes; called once who is signed
// in is known (from the server, or from the offline copy).
export async function startSync(user: Me) {
  me = user;
  await refreshCounts();
  if (!isOffline()) await sync();
}

export function stopSync() {
  me = null;
  clearTimeout(retry);
  setState({ pendingIds: new Set(), pendingCount: 0, rejectedCount: 0 });
}

// What to do once a sync has had changes refused, so what's on screen no
// longer matches the server: load the page's data again.
export function setOnRejected(callback: () => void) {
  onRejected = callback;
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
// when offline (or while earlier ones are still waiting, so they reach the
// server in order). Resolves false only when an online send fails, which
// the caller treats as it always has: the change didn't happen.
export async function submit(change: Change): Promise<boolean> {
  if (me && (isOffline() || state.pendingCount > 0)) {
    await addPending(me.id, change);
    await applyChange(me, change);
    await refreshCounts();
    void sync();
    return true;
  }

  const { url, init } = toRequest(change);
  let res: Response;
  try {
    res = await apiFetch(url, init);
  } catch {
    return false;
  }
  if (!res.ok) return false;
  if (me) await applyChange(me, change);
  return true;
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
  let refused = 0;
  let failed = false;
  setState({ syncing: true });

  try {
    // Takes the oldest change each time round, so ones made while this
    // runs are sent too, after the ones before them.
    while (!isOffline() && me === user) {
      const pending = await nextPending(user.id);
      if (!pending) break;

      const { url, init } = toRequest(pending.change);
      let res: Response;
      try {
        res = await apiFetch(url, init);
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
        refused++;
      }
      await refreshCounts();
    }
  } finally {
    setState({ syncing: false });
  }

  if (me !== user) return;
  if (failed) retryLater();
  else retryDelay = FIRST_RETRY;
  if (refused > 0) onRejected();
}

window.addEventListener("online", () => {
  setState({ offline: false });
  void sync();
});
window.addEventListener("offline", () => setState({ offline: true }));
