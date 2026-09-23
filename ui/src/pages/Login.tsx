import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import { Link, useNavigate } from "react-router";

export default function Login() {
  const [email, setEmail] = useState("");
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    sessionStorage.setItem("email", email);
    fetch("/api/auth/send", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ email }),
    });

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
              placeholder="Enter your email"
              className="w-full"
              onChange={(e) => setEmail(e.target.value)}
              value={email}
            />
            <Button className="w-full" asChild type="submit">
              <Link to="/verify">Continue</Link>
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
