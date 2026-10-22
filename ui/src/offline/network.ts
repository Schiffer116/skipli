import { useSyncExternalStore } from "react";

// Offline means the browser says so. A server that can't be reached while
// the browser is online keeps changes waiting all the same, and loaders
// read the Offline copy then too.
export const isOffline = () => !navigator.onLine;

function subscribe(listener: () => void) {
  window.addEventListener("online", listener);
  window.addEventListener("offline", listener);
  return () => {
    window.removeEventListener("online", listener);
    window.removeEventListener("offline", listener);
  };
}

export function useOffline() {
  return useSyncExternalStore(subscribe, isOffline);
}
