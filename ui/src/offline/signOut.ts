import { deleteCopy } from "./copy";
import {
  activeUser,
  setActiveUser,
  setSignOutPending,
  signOutPending,
} from "./session";
import { isOffline, stopSync } from "./sync";

// Signs out on this device: deletes this person's Offline copy, including
// Pending and Rejected changes (the caller confirms that first), and ends
// the session. Offline the server can't clear the session cookie, so that
// is left to finishSignOut once the device is back online.
export async function signOut() {
  const userId = activeUser();
  stopSync();
  setActiveUser(null);
  if (userId) await deleteCopy(userId);

  setSignOutPending(true);
  await finishSignOut();
}

// Ends a session that was signed out while offline. Resolves true if it
// did, so the caller can go to sign-in instead of loading a dead session.
export async function finishSignOut() {
  if (!signOutPending() || isOffline()) return false;
  try {
    const res = await fetch("/api/auth/signout", { method: "POST" });
    if (!res.ok) return false;
  } catch {
    return false;
  }
  setSignOutPending(false);
  return true;
}
