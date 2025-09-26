import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import { useNavigate } from "react-router";

export default function Verify() {
  const navigate = useNavigate();
  const [code, setCode] = useState("");

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const email = sessionStorage.getItem("email");

    const data = await fetch("/api/auth/verify", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ email, code }),
    });

    const res = await data.json();
    localStorage.setItem("accessToken", res.accessToken);
    navigate("/boards");
  };

  return (
    <div className="w-full min-h-screen bg-gradient-to-br from-orange-50 to-orange-100 flex justify-center items-center text-center">
      <Card className="bg-card border-border shadow-sm w-full max-w-md">
        <CardHeader>
          <CardTitle>Email Verification</CardTitle>
          <CardDescription>
            Please enter the verification code sent to your email
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <form onSubmit={handleSubmit} className="space-y-3">
            <Input
              placeholder="Enter verification code"
              className="w-full"
              onChange={(e) => setCode(e.target.value)}
            />
            <Button className="w-full" type="submit">
              Verify
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
