import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

export default function Login() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (submitting) return;

    setSubmitting(true);
    const res = await fetch("/api/auth/send", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ email }),
    }).finally(() => setSubmitting(false));
    if (res.status === 400) {
      setError("That email address can't receive mail. Check it for typos.");
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
        </CardContent>
      </Card>
    </div>
  );
}
