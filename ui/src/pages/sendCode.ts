export type SendCodeResult =
  { ok: true; session: string } | { ok: false; message: string };

/** Asks for a sign-in code by email; failures come back as user-facing text. */
export async function sendCode(email: string): Promise<SendCodeResult> {
  const res = await fetch("/api/auth/send", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ email }),
  }).catch(() => null);
  if (!res) {
    return {
      ok: false,
      message: navigator.onLine
        ? "Couldn't reach Skipli. Please try again."
        : "You're offline. Connect to the internet to sign in.",
    };
  }
  if (res.status === 400) {
    return {
      ok: false,
      message: "That email address can't receive mail. Check it for typos.",
    };
  }
  if (res.status === 429) {
    return {
      ok: false,
      message: "A code was just sent. Wait a minute before requesting another.",
    };
  }
  if (!res.ok) {
    return { ok: false, message: "Couldn't send the code. Please try again." };
  }
  const { session } = await res.json();
  return { ok: true, session };
}
