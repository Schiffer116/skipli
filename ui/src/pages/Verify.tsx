import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CircleAlert, Loader2, Mail } from "lucide-react";
import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router";

import AuthShell from "@/components/AuthShell";
import { setSignOutPending } from "@/offline/session";
import { forgetMe } from "./Boards/loaders";
import { sendCode } from "./sendCode";

const CODE_LENGTH = 8;

type VerifyError = { title: string; message: string };

export default function Verify() {
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const [email] = useState(() => sessionStorage.getItem("email"));
  const [session, setSession] = useState(() =>
    sessionStorage.getItem("session"),
  );
  const [code, setCode] = useState("");
  const [error, setError] = useState<VerifyError | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [resent, setResent] = useState(false);

  const fail = (error: VerifyError) => {
    setError(error);
    // Select the wrong code so typing replaces it.
    inputRef.current?.focus();
    inputRef.current?.select();
  };

  const handleResend = async () => {
    if (!email) {
      fail({
        title: "Session expired",
        message: "Go back and enter your email again to get a new code.",
      });
      return;
    }

    setResending(true);
    setError(null);
    const result = await sendCode(email).finally(() => setResending(false));
    if (!result.ok) {
      setResent(false);
      fail({ title: "Couldn't resend the code", message: result.message });
      return;
    }
    sessionStorage.setItem("session", result.session);
    setSession(result.session);
    setCode("");
    setResent(true);
    inputRef.current?.focus();
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    if (!email || !session) {
      fail({
        title: "Session expired",
        message: "Go back and enter your email again to get a new code.",
      });
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const res = await fetch("/api/auth/verify", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ email, code: code.trim(), session }),
      });

      if (res.status === 401) {
        fail({
          title: "Incorrect verification code",
          message:
            "That code doesn't match the one we sent. Check the latest email and try again.",
        });
        return;
      }
      if (!res.ok) {
        fail({
          title: "Couldn't verify your code",
          message: "Something went wrong on our side. Please try again.",
        });
        return;
      }

      // A new session replaces any left over from signing out offline.
      setSignOutPending(false);
      forgetMe();
      navigate("/boards");
    } catch {
      fail({
        title: "Couldn't reach the server",
        message: "Check your connection and try again.",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthShell>
      <div className="flex flex-col gap-3">
        <h2 className="text-[34px] leading-tight font-bold tracking-tight text-nav-foreground">
          Check your email
        </h2>
        <p className="text-nav-muted">
          Enter the {CODE_LENGTH}-digit code we just sent you.
        </p>
      </div>
      {email && (
        <div className="flex items-center gap-3 rounded-lg border bg-muted px-3 py-2.5">
          <Mail className="size-5 shrink-0 text-muted-foreground" aria-hidden />
          <p className="min-w-0 flex-1">
            <span className="block text-xs text-muted-foreground">
              Code sent to
            </span>
            <span className="block text-sm font-bold [overflow-wrap:anywhere]">
              {email}
            </span>
          </p>
          <Link to="/login" className="text-sm font-bold text-link">
            Change
          </Link>
        </div>
      )}
      {error && (
        <div
          id="code-error"
          role="alert"
          className="flex gap-3 rounded-md border-2 border-destructive bg-destructive/10 px-3 py-2"
        >
          <CircleAlert
            className="size-5 shrink-0 text-destructive"
            aria-hidden
          />
          <div className="space-y-1">
            <p className="font-bold text-destructive">{error.title}</p>
            <p className="text-sm">{error.message}</p>
            {error.title === "Session expired" && (
              <Link to="/login" className="text-sm font-bold text-link">
                Back to sign in
              </Link>
            )}
          </div>
        </div>
      )}
      <form onSubmit={handleSubmit} className="flex flex-col gap-3">
        <Label htmlFor="code">Verification code</Label>
        <Input
          id="code"
          ref={inputRef}
          placeholder={"0".repeat(CODE_LENGTH)}
          className="h-14 text-center text-2xl font-semibold tracking-[0.4em] aria-invalid:border-2 md:text-2xl"
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={CODE_LENGTH}
          autoFocus
          aria-invalid={error !== null}
          aria-describedby={error ? "code-error" : undefined}
          value={code}
          onChange={(e) => {
            setCode(e.target.value.replace(/\D/g, "").slice(0, CODE_LENGTH));
            setResent(false);
          }}
        />
        <Button
          className="mt-2 h-12 text-base font-bold"
          type="submit"
          disabled={submitting}
        >
          {submitting && <Loader2 className="animate-spin" />}
          {submitting ? "Verifying…" : "Verify"}
        </Button>
      </form>
      <p className="text-center text-sm text-nav-muted">
        {resent ? "A new code is on its way." : "Didn't get it?"}{" "}
        <button
          type="button"
          onClick={handleResend}
          disabled={resending}
          className="rounded-sm px-0.5 py-1 font-bold text-link outline-none hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:opacity-50"
        >
          {resending ? "Sending…" : "Resend code"}
        </button>
      </p>
    </AuthShell>
  );
}
