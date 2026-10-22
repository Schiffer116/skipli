import { useEffect } from "react";
import { Link, useRevalidator } from "react-router";
import { CloudOff, LogIn, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { isBoardChange } from "@/offline/changes";
import { useOffline } from "@/offline/network";
import { onDropped, seenDropped, usePending } from "@/offline/pending";

const changes = (n: number) => `${n} ${n === 1 ? "change" : "changes"}`;

// Says when the app is working from the Offline copy, how many changes
// haven't reached the server, and when they can't be sent until this
// person signs in again. Dropped changes are told of in a toast.
export default function OfflineBanner() {
  const offline = useOffline();
  const { queued, waiting, sending, paused } = usePending();
  useToldOfDrops();

  return (
    <div role="status" aria-live="polite">
      {offline ? (
        <Bar Icon={CloudOff}>
          You're offline. {queued > 0 && `${changes(queued)} waiting to sync. `}
          Boards shared with others are read-only until you reconnect.
        </Bar>
      ) : paused ? (
        <Bar Icon={LogIn} tone="warning">
          <Link to="/login" className="font-medium underline">
            Sign in again
          </Link>{" "}
          to send {queued} waiting {queued === 1 ? "change" : "changes"}.
        </Bar>
      ) : (
        waiting > 0 && (
          <Bar Icon={RefreshCw} spin={sending}>
            {sending
              ? `Syncing ${changes(queued)}…`
              : `${changes(queued)} waiting to sync. They'll be sent when Skipli can be reached.`}
          </Bar>
        )
      )}
    </div>
  );
}

// Tells the person how many of their changes were dropped, in one toast
// that counts up while more are, and loads the board list again when one
// was to it. An open board catches up on its own.
function useToldOfDrops() {
  const { dropped } = usePending();
  const { revalidate } = useRevalidator();

  useEffect(
    () =>
      onDropped((change) => {
        if (!isBoardChange(change)) void revalidate();
      }),
    [revalidate],
  );

  useEffect(() => {
    if (dropped === 0) return;
    toast.error(`${changes(dropped)} couldn't be saved`, {
      id: "dropped",
      onDismiss: seenDropped,
      onAutoClose: seenDropped,
    });
  }, [dropped]);
}

function Bar({
  Icon,
  spin = false,
  tone = "info",
  children,
}: {
  Icon: typeof CloudOff;
  spin?: boolean;
  tone?: "info" | "warning";
  children: React.ReactNode;
}) {
  return (
    <div
      className={`flex items-center gap-2 px-4 py-2 text-sm border-b ${tone === "warning" ? "bg-destructive/10 border-destructive/30" : "bg-muted border-border"}`}
    >
      <Icon className={`size-4 shrink-0 ${spin ? "animate-spin" : ""}`} />
      <p className="min-w-0">{children}</p>
    </div>
  );
}
