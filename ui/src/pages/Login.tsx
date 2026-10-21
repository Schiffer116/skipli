import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";

import GoogleButton from "@/components/GoogleButton";
import { finishSignOut } from "@/offline/signOut";

// Why Google sign-in sent the browser back here, by the API's error code.
const googleErrors: Record<string, string> = {
  google: "Couldn't sign in with Google. Please try again.",
  google_cancelled: "Google sign-in was cancelled.",
};

export default function Login() {
  const [searchParams] = useSearchParams();
  const [email, setEmail] = useState("");
  const [error, setError] = useState(
    () => googleErrors[searchParams.get("error") ?? ""] ?? "",
  );
  const [submitting, setSubmitting] = useState(false);
  const [google, setGoogle] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const navigate = useNavigate();

  // Google sign-in is on per environment, so ask the API whether to offer it.
  useEffect(() => {
    fetch("/api/auth/providers")
      .then((res) => (res.ok ? res.json() : null))
      .then((providers) => setGoogle(providers?.google === true))
      .catch(() => {});
    // Coming back from Google with the back button restores this page as
    // it was left, still busy.
    const onPageShow = (e: PageTransitionEvent) => {
      if (e.persisted) setLeaving(false);
    };
    window.addEventListener("pageshow", onPageShow);
    return () => window.removeEventListener("pageshow", onPageShow);
  }, []);

  const signInWithGoogle = async () => {
    setLeaving(true);
    // End a session signed out while offline before starting a new one.
    await finishSignOut();
    window.location.assign("/api/auth/google");
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (submitting) return;

    setSubmitting(true);
    // End a session signed out while offline before starting a new one.
    await finishSignOut();
    const res = await fetch("/api/auth/send", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ email }),
    })
      .catch(() => null)
      .finally(() => setSubmitting(false));
    if (!res) {
      setError(
        navigator.onLine
          ? "Couldn't reach Skipli. Please try again."
          : "You're offline. Connect to the internet to sign in.",
      );
      return;
    }
    if (res.status === 400) {
      setError("That email address can't receive mail. Check it for typos.");
      return;
    }
    if (res.status === 429) {
      setError(
        "A code was just sent. Wait a minute before requesting another.",
      );
      return;
    }
    if (!res.ok) {
      setError("Couldn't send the code. Please try again.");
      return;
    }

    const { session } = await res.json();
    sessionStorage.setItem("email", email);
    sessionStorage.setItem("session", session);
    navigate("/verify");
  };

  return (
    <div className="w-full min-h-screen bg-background border-t-[3rem] border-nav flex justify-center items-center text-center">
      <Card className="w-full max-w-md border-t-4 border-t-primary">
        <CardHeader>
          <img src="skipli.png" alt="Logo" className="h-20 w-auto mx-auto" />
          <CardDescription>Enter your email to continue</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <form onSubmit={handleSubmit} className="space-y-3">
            <Input
              type="email"
              required
              autoComplete="email"
              placeholder="Enter your email"
              className="w-full"
              aria-invalid={error !== ""}
              onChange={(e) => {
                setEmail(e.target.value);
                setError("");
              }}
              value={email}
            />
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <Button className="w-full" type="submit" disabled={submitting}>
              {submitting && <Loader2 className="animate-spin" />}
              {submitting ? "Sending code…" : "Continue"}
            </Button>
          </form>
          {google && (
            <>
              <div className="flex items-center gap-3 text-xs text-muted-foreground">
                <span className="h-px flex-1 bg-border" />
                or
                <span className="h-px flex-1 bg-border" />
              </div>
              <GoogleButton busy={leaving} onClick={signInWithGoogle} />
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
