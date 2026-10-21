import { useEffect } from "react";
import { useNavigate } from "react-router";

import { setSignOutPending } from "@/offline/session";
import { forgetMe } from "./Boards/loaders";

// Where the API sends the browser once Google sign-in has set the session
// cookies: the same hand-off Verify does after an email code.
export default function SignedIn() {
  const navigate = useNavigate();

  useEffect(() => {
    // A new session replaces any left over from signing out offline.
    setSignOutPending(false);
    forgetMe();
    navigate("/boards", { replace: true });
  }, [navigate]);

  return null;
}
