// Who the offline copy on this device belongs to: the last person signed in
// here. Offline, nothing can ask the server, so this is how the app knows
// whose boards to show.
const USER_KEY = "skipli:offlineUser";

// Set by signing out offline: the server hasn't cleared the session cookie
// yet, so the app does that first thing once it's back online.
const SIGN_OUT_KEY = "skipli:signOutPending";

function read(key: string) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Storage blocked: offline access just won't know who was signed in.
  }
}

export const activeUser = () => read(USER_KEY);
export const setActiveUser = (id: string | null) => write(USER_KEY, id);

export const signOutPending = () => read(SIGN_OUT_KEY) === "true";
export const setSignOutPending = (pending: boolean) =>
  write(SIGN_OUT_KEY, pending ? "true" : null);

// Guest mode, on this browser: "active" while someone uses Skipli without an
// account (their boards are an Offline copy that is never sent), "moving"
// once they've signed in and their boards are still to move into the
// account. Absent otherwise.
const GUEST_KEY = "skipli:guest";

export type GuestState = "active" | "moving";

export const guestState = (): GuestState | null => {
  const value = read(GUEST_KEY);
  return value === "active" || value === "moving" ? value : null;
};
export const setGuestState = (state: GuestState | null) =>
  write(GUEST_KEY, state);

export const isGuest = () => guestState() === "active";
