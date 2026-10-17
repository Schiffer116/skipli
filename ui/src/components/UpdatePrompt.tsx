import { useRegisterSW } from "virtual:pwa-register/react";

import { Button } from "@/components/ui/button";

// Offers a new version of the app once it has downloaded, rather than
// reloading on its own, which could lose an open dialog or a half-typed
// task. Left alone, the new version starts the next time the app opens.
export default function UpdatePrompt() {
  const {
    needRefresh: [needRefresh, setNeedRefresh],
    updateServiceWorker,
  } = useRegisterSW();

  if (!needRefresh) return null;

  return (
    <div
      role="status"
      className="fixed bottom-4 right-4 z-50 flex items-center gap-3 rounded-lg border bg-card px-4 py-3 text-sm shadow-lg"
    >
      <p>A new version of Skipli is available.</p>
      <Button variant="ghost" size="sm" onClick={() => setNeedRefresh(false)}>
        Later
      </Button>
      <Button size="sm" onClick={() => updateServiceWorker()}>
        Reload
      </Button>
    </div>
  );
}
