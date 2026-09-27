import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { CircleAlert, Loader2, Mail } from "lucide-react";
import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router";

type VerifyError = { title: string; message: string };

export default function Verify() {
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const [email] = useState(() => sessionStorage.getItem("email"));
  const [code, setCode] = useState("");
  const [error, setError] = useState<VerifyError | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const fail = (error: VerifyError) => {
    setError(error);
    // Select the wrong code so typing replaces it.
    inputRef.current?.focus();
    inputRef.current?.select();
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    if (!email) {
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
        body: JSON.stringify({ email, code: code.trim() }),
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

      const { accessToken } = await res.json();
      localStorage.setItem("accessToken", accessToken);
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
    <div className="w-full min-h-screen bg-background border-t-[3rem] border-nav flex justify-center items-center text-center">
      <Card className="w-full max-w-md border-t-4 border-t-primary">
        <CardHeader>
          <CardTitle>Email Verification</CardTitle>
          <CardDescription>
            Enter the 6-digit code we emailed you
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Same shape as the error box below, so the two read as a pair:
              where the code went, then (if needed) what went wrong. */}
          {email && (
            <div className="flex items-center gap-3 rounded-md border bg-muted px-3 py-2 text-left">
              <Mail
                className="size-5 shrink-0 text-muted-foreground"
                aria-hidden
              />
              <p className="min-w-0 flex-1 text-sm">
                <span className="block text-xs text-muted-foreground">
                  Code sent to
                </span>
                <span className="block font-bold [overflow-wrap:anywhere]">
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
              role="alert"
              className="flex gap-3 rounded-md border-2 border-destructive bg-destructive/10 px-3 py-2 text-left"
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
          <form onSubmit={handleSubmit} className="space-y-3">
            <Input
              ref={inputRef}
              placeholder="Enter verification code"
              className="w-full aria-invalid:border-2"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              aria-invalid={error !== null}
              aria-label="Verification code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            <Button className="w-full" type="submit" disabled={submitting}>
              {submitting && <Loader2 className="animate-spin" />}
              {submitting ? "Verifying…" : "Verify"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
