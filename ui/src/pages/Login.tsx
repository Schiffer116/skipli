import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

import AuthShell from "@/components/AuthShell";
import { finishSignOut } from "@/offline/signOut";
import { sendCode } from "./sendCode";

export default function Login() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (submitting) return;

    setSubmitting(true);
    // End a session signed out while offline before starting a new one.
    await finishSignOut();
    const result = await sendCode(email).finally(() => setSubmitting(false));
    if (!result.ok) {
      setError(result.message);
      return;
    }

    sessionStorage.setItem("email", email);
    sessionStorage.setItem("session", result.session);
    navigate("/verify");
  };

  return (
    <AuthShell>
      <div className="flex flex-col gap-3">
        <h2 className="text-[34px] leading-tight font-bold tracking-tight text-nav-foreground">
          Sign in to Skipli
        </h2>
        <p className="text-nav-muted">
          Enter your email and we'll send you an 8-digit code.
        </p>
      </div>
      <form onSubmit={handleSubmit} className="flex flex-col gap-3">
        <Label htmlFor="email">Email</Label>
        <Input
          id="email"
          type="email"
          required
          autoComplete="email"
          placeholder="you@company.com"
          className="h-12 text-base"
          aria-invalid={error !== ""}
          aria-describedby={error ? "email-error" : undefined}
          onChange={(e) => {
            setEmail(e.target.value);
            setError("");
          }}
          value={email}
        />
        {error && (
          <p id="email-error" role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Button
          className="mt-2 h-12 text-base font-bold"
          type="submit"
          disabled={submitting}
        >
          {submitting && <Loader2 className="animate-spin" />}
          {submitting ? "Sending code…" : "Continue"}
        </Button>
        <p className="text-center text-sm text-nav-muted">
          New here? Your account is created when you sign in.
        </p>
      </form>
    </AuthShell>
  );
}
