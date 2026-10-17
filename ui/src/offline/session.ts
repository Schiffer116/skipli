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
