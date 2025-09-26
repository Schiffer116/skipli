import { useEffect, useState } from "react";
import { Check, Loader2, XCircle } from "lucide-react";
import { useSearchParams } from "react-router";

export default function SpinnerToCheckPage() {
  const [status, setStatus] = useState<"loading" | "success" | "failure">(
    "loading",
  );
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token");

  useEffect(() => {
    (async () => {
      if (!token) {
        setStatus("failure");
        return;
      }

      const res = await fetch(`/api/boards/invite/accept`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
        },
        body: JSON.stringify({
          token,
        }),
      });

      if (!res.ok) {
        setStatus("failure");
        return;
      }

      setStatus("success");
    })();
  }, []);

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="flex items-center gap-4">
        {status === "loading" ? (
          <Loader2 className="h-16 w-16 animate-spin text-orange-500" />
        ) : status === "success" ? (
          <Check className="h-16 w-16 text-green-600" />
        ) : (
          <XCircle className="h-16 w-16 text-red-600" />
        )}
        <div>
          <p className="text-lg font-semibold text-gray-800">
            {status ? "Done!" : "Loading..."}
          </p>
          <p className="">
            {status === "loading"
              ? "Accepting invite"
              : status === "success"
                ? "Redirecting you to the invited board"
                : "Error accepting invite"}
          </p>
        </div>
      </div>
    </div>
  );
}
