import { useSyncExternalStore } from "react";

import type { Me } from "@/pages/Boards/loaders";
import { apiFetch } from "@/utils/apiFetch";

import { alreadySaved, toRequest, touchedIds, type Change } from "./changes";
import { pendingStore, type PendingChange } from "./copy";
import { newId } from "./ids";
import { isOffline } from "./network";

// What became of a change, for whoever made it: the server has it, it
// waits as a Pending change (and may still be dropped later), or the
// server couldn't apply it and it was dropped.
export type Outcome = "accepted" | "waiting" | "dropped";

// Sends one change over HTTP under its change id; throws when no answer
// comes back at all.
export type Transport = (change: Change, changeId: string) => Promise<Response>;

// Where Pending changes are kept: queueing one applies it to the Offline
// copy in the same transaction, and dropping one marks what it touched
// as out of date there.
export type PendingStore = {
  queue: (change: Change, id: string) => Promise<void>;
  all: () => Promise<PendingChange[]>;
  remove: (seq: number) => Promise<void>;
  drop: (pending: PendingChange) => Promise<void>;
};

export type PendingState = {
  // Every change the server hasn't accepted yet, the one being sent too.
  queued: number;
  // Those known to wait: the server couldn't take them yet, or earlier
  // ones are holding them up. A change on its first way out isn't one.
  waiting: number;
  // Boards, cards and tasks with waiting changes.
  waitingIds: Set<string>;
  sending: boolean;
  // The server said this person's session is over: nothing is sent until
  // they sign in again.
  paused: boolean;
  // Changes dropped since the person was last told.
  dropped: number;
};

export type Pending = {
  userId: string;
  // Queues a change (applying it to the Offline copy), then sends it after
  // the ones made before it. Resolves once its first attempt is answered,
  // or at once when the queue is held.
  make: (change: Change) => Promise<Outcome>;
  // Sends what's queued now, unless offline or paused; resolves once the
  // queue is empty or held.
  send: () => Promise<void>;
  // Resolves once every change made so far is queued and sending has had
  // its go.
  settled: () => Promise<void>;
  // Whether a broadcast with this change id is one of this person's own.
  isOwn: (changeId: string) => boolean;
  // Sends again after a pause, once the same person has signed in again.
  resume: () => void;
  // Calls back with each change dropped, whenever that happens.
  onDropped: (listener: (change: Change) => void) => () => void;
  // Starts the dropped count again, once the person has been told.
  seenDropped: () => void;
  state: () => PendingState;
  subscribe: (listener: () => void) => () => void;
  close: () => void;
};

// After no answer, a server error or too many requests, the queue tries
// again on its own, waiting longer each time in a row.
const FIRST_RETRY = 5_000;
const LAST_RETRY = 5 * 60_000;

// Accepted changes' ids are kept a while, as the server's broadcast of one
// can come after its answer.
const ACCEPTED_KEPT = 500;

type Answer = "accepted" | "retry" | "paused" | "dropped";

// The one reading of what the server answered to a change.
function classify(change: Change, res: Response | null): Answer {
  if (!res || res.status >= 500 || res.status === 429) return "retry";
  if (res.ok || alreadySaved(change, res.status)) return "accepted";
  // apiFetch has already tried to refresh the session.
  if (res.status === 401) return "paused";
  return "dropped";
}

// One signed-in person's Pending changes: every change they make goes
// through here, is kept first, and is sent in the order made, one at a
// time, across all boards and the board list.
export function createPending({
  userId,
  transport,
  store,
}: {
  userId: string;
  transport: Transport;
  store: PendingStore;
}): Pending {
  let state: PendingState = {
    queued: 0,
    waiting: 0,
    waitingIds: new Set(),
    sending: false,
    paused: false,
    dropped: 0,
  };
  const listeners = new Set<() => void>();
  const droppedListeners = new Set<(change: Change) => void>();
  const setState = (update: Partial<PendingState>) => {
    state = { ...state, ...update };
    listeners.forEach((listener) => listener());
  };

  let closed = false;
  // Changes made, by id, still waiting to hear their outcome.
  const outcomes = new Map<string, (outcome: Outcome) => void>();
  // Queued changes known to be waiting (see PendingState.waiting).
  const waiting = new Set<string>();
  const queuedIds = new Set<string>();
  const accepted = new Set<string>();

  const refresh = async () => {
    const all = await store.all();
    queuedIds.clear();
    all.forEach(({ id }) => queuedIds.add(id));
    const held = all.filter(({ id }) => waiting.has(id));
    setState({
      queued: all.length,
      waiting: held.length,
      waitingIds: new Set(held.flatMap(({ change }) => touchedIds(change))),
    });
  };

  const resolve = (id: string, outcome: Outcome) => {
    outcomes.get(id)?.(outcome);
    outcomes.delete(id);
  };

  // Whatever is queued now waits: the queue is held.
  const hold = async () => {
    const all = await store.all();
    all.forEach(({ id }) => waiting.add(id));
    await refresh();
    all.forEach(({ id }) => resolve(id, "waiting"));
  };

  let retryDelay = FIRST_RETRY;
  let retry: ReturnType<typeof setTimeout> | undefined;
  const retryLater = () => {
    clearTimeout(retry);
    retry = setTimeout(() => void send(), retryDelay);
    retryDelay = Math.min(retryDelay * 2, LAST_RETRY);
  };
  const backingOff = () => retry !== undefined;

  // One run at a time. Asked to send while one runs, it runs again once
  // done, so a change queued as the last one finished isn't missed. A new
  // change doesn't cut a wait to try again short; being back online or a
  // page loading does.
  let running: Promise<void> | null = null;
  let again: "now" | "unless backing off" | null = null;
  const sendWhen = (when: NonNullable<typeof again>) => {
    if (running) {
      if (again !== "now") again = when;
      return running;
    }
    running = (async () => {
      do {
        again = null;
        await run();
      } while (
        !closed &&
        (again === "now" || (again === "unless backing off" && !backingOff()))
      );
    })().finally(() => (running = null));
    return running;
  };
  const send = () => sendWhen("now");

  async function run() {
    clearTimeout(retry);
    retry = undefined;
    if (closed || state.paused || isOffline()) return hold();
    setState({ sending: true });
    try {
      // Takes the oldest change each time round, so ones made while this
      // runs are sent too, after the ones before them.
      for (;;) {
        if (closed) return;
        if (isOffline()) return hold();
        const [next] = await store.all();
        if (!next) break;

        let res: Response | null;
        try {
          res = await transport(next.change, next.id);
        } catch {
          res = null;
        }
        if (closed) return;

        const answer = classify(next.change, res);
        if (answer === "retry") {
          retryLater();
          return hold();
        }
        if (answer === "paused") {
          setState({ paused: true });
          return hold();
        }

        retryDelay = FIRST_RETRY;
        waiting.delete(next.id);
        if (answer === "accepted") {
          await store.remove(next.seq);
          accepted.add(next.id);
          if (accepted.size > ACCEPTED_KEPT) {
            accepted.delete(accepted.values().next().value!);
          }
          resolve(next.id, "accepted");
        } else {
          await store.drop(next);
          setState({ dropped: state.dropped + 1 });
          resolve(next.id, "dropped");
          droppedListeners.forEach((listener) => listener(next.change));
        }
        await refresh();
      }
    } finally {
      if (!closed) setState({ sending: false });
    }
  }

  // Changes are queued one after another, so the queue's order is the
  // order they were made in.
  let queueing: Promise<unknown> = Promise.resolve();

  const online = () => void send();
  window.addEventListener("online", online);

  // Changes left from before (an earlier visit, or another tab) wait
  // already.
  void store
    .all()
    .then((all) => all.forEach(({ id }) => waiting.add(id)))
    .then(refresh)
    .then(() => send());

  return {
    userId,

    make(change) {
      const id = newId();
      const outcome = new Promise<Outcome>((resolve) =>
        outcomes.set(id, resolve),
      );
      queuedIds.add(id);
      queueing = queueing
        .then(() => store.queue(change, id))
        .then(refresh)
        // Behind a change waiting to be tried again, it waits too.
        .then(() =>
          backingOff() ? hold() : void sendWhen("unless backing off"),
        )
        // The device couldn't keep it, so it never happened.
        .catch(() => resolve(id, "dropped"));
      return outcome;
    },

    send,

    async settled() {
      await queueing;
      await running;
    },

    isOwn: (changeId) => queuedIds.has(changeId) || accepted.has(changeId),

    resume() {
      if (!state.paused) return;
      setState({ paused: false });
      void send();
    },

    onDropped(listener) {
      droppedListeners.add(listener);
      return () => void droppedListeners.delete(listener);
    },

    seenDropped: () => setState({ dropped: 0 }),

    state: () => state,

    subscribe(listener) {
      listeners.add(listener);
      return () => void listeners.delete(listener);
    },

    close() {
      closed = true;
      clearTimeout(retry);
      window.removeEventListener("online", online);
      outcomes.forEach((resolve) => resolve("waiting"));
      outcomes.clear();
    },
  };
}

// Sends a change as its own HTTP request, under its change id, which the
// server's broadcast of it carries.
export const httpTransport: Transport = (change, changeId) => {
  const { url, init } = toRequest(change);
  return apiFetch(url, {
    ...init,
    headers: { ...init.headers, "X-Change-Id": changeId },
  });
};

let current: Pending | null = null;
let stopForwarding = () => {};
const droppedListeners = new Set<(change: Change) => void>();

const signedOut: PendingState = {
  queued: 0,
  waiting: 0,
  waitingIds: new Set(),
  sending: false,
  paused: false,
  dropped: 0,
};
const swapListeners = new Set<() => void>();

// Starts sending this person's Pending changes, once who is signed in is
// known. The same person again keeps theirs going, and sends again after
// a pause; someone else leaves the last person's changes waiting on the
// device, untouched, until they sign in here again.
export function startPending(me: Me) {
  if (current?.userId === me.id) {
    current.resume();
    return current;
  }
  current?.close();
  stopForwarding();
  current = createPending({
    userId: me.id,
    transport: httpTransport,
    store: pendingStore(me),
  });
  stopForwarding = current.onDropped((change) =>
    droppedListeners.forEach((listener) => listener(change)),
  );
  swapListeners.forEach((listener) => listener());
  return current;
}

// Stops sending, at sign-out.
export function stopPending() {
  current?.close();
  stopForwarding();
  current = null;
  swapListeners.forEach((listener) => listener());
}

// The signed-in person's Pending changes.
export function pending(): Pending {
  if (!current) throw new Error("Nobody is signed in");
  return current;
}

// Makes a change as the signed-in person.
export const make = (change: Change) => pending().make(change);

// Calls back with each change the signed-in person had dropped, whoever
// that is by then.
export function onDropped(listener: (change: Change) => void) {
  droppedListeners.add(listener);
  return () => void droppedListeners.delete(listener);
}

// The signed-in person has been told of their dropped changes.
export const seenDropped = () => current?.seenDropped();

// Whether a broadcast carrying this change id is the signed-in person's own.
export const isOwnChange = (changeId: string) =>
  current?.isOwn(changeId) ?? false;

export function usePending(): PendingState {
  return useSyncExternalStore(
    (listener) => {
      // Follows whoever is signed in, as that changes.
      let stop = current?.subscribe(listener);
      const swapped = () => {
        stop?.();
        stop = current?.subscribe(listener);
        listener();
      };
      swapListeners.add(swapped);
      return () => {
        swapListeners.delete(swapped);
        stop?.();
      };
    },
    () => current?.state() ?? signedOut,
  );
}
