import { useEffect, useState } from "react";
import { CloudOff, RefreshCw, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { loadRejectedSeen, saveRejectedSeen } from "@/offline/copy";
import { activeUser } from "@/offline/session";
import { useSync } from "@/offline/sync";

const changes = (n: number) => `${n} ${n === 1 ? "change" : "changes"}`;

// Says when the app is working from the Offline copy, how many changes
// haven't reached the server, and when some were refused. Refused ones are
// kept on the device; reviewing them is still to come.
export default function OfflineBanner() {
  const { offline, syncing, pendingCount, rejectedCount } = useSync();
  const [rejectedSeen, setRejectedSeen] = useState<number | null>(null);

  useEffect(() => {
    const userId = activeUser();
    if (userId) void loadRejectedSeen(userId).then(setRejectedSeen);
  }, []);

  const unseenRejected =
    rejectedSeen === null ? 0 : Math.max(0, rejectedCount - rejectedSeen);
  const dismissRejected = () => {
    const userId = activeUser();
    setRejectedSeen(rejectedCount);
    if (userId) void saveRejectedSeen(userId, rejectedCount);
  };

  return (
    <div role="status" aria-live="polite">
      {offline ? (
        <Bar Icon={CloudOff}>
          You're offline.{" "}
          {pendingCount > 0 && `${changes(pendingCount)} waiting to sync. `}
          Boards shared with others are read-only until you reconnect.
        </Bar>
      ) : (
        pendingCount > 0 && (
          <Bar Icon={RefreshCw} spin={syncing}>
            {syncing
              ? `Syncing ${changes(pendingCount)}…`
              : `${changes(pendingCount)} waiting to sync. They'll be sent when Skipli can be reached.`}
          </Bar>
        )
      )}
      {unseenRejected > 0 && (
        <Bar Icon={TriangleAlert} tone="warning">
          {changes(unseenRejected)} made offline couldn't be saved.{" "}
          {unseenRejected === 1 ? "It's" : "They're"} kept on this device.
          <Button
            variant="link"
            size="sm"
            className="h-auto p-0 ml-2"
            onClick={dismissRejected}
          >
            Dismiss
          </Button>
        </Bar>
      )}
    </div>
  );
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
