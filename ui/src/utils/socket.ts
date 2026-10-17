import { apiFetch } from "./apiFetch";

type Handler = (...args: never[]) => void;

const handlers = new Map<string, Set<Handler>>();
let ws: WebSocket | null = null;
let boardId: string | null = null;
let retry: ReturnType<typeof setTimeout> | undefined;
let waitForOnline: (() => void) | null = null;

function open(id: string) {
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  const conn = new WebSocket(
    `${protocol}://${location.host}/api/ws?boardId=${encodeURIComponent(id)}`,
  );

  conn.onmessage = (e) => {
    const { event, args } = JSON.parse(e.data);
    handlers
      .get(event)
      ?.forEach((handler) =>
        (handler as (...args: unknown[]) => void)(...args),
      );
  };

  conn.onclose = () => {
    if (ws !== conn) return;
    const reconnect = async () => {
      try {
        // Refreshes the session if it expired while disconnected.
        await apiFetch("/api/me");
      } catch {
        // Still unreachable: the new connection fails and retries again.
      }
      if (ws === conn) open(id);
    };

    // Offline there's nothing to reach, so wait for the browser to say the
    // connection is back instead of retrying every few seconds.
    if (navigator.onLine) {
      retry = setTimeout(reconnect, 3000);
    } else {
      waitForOnline = () => {
        waitForOnline = null;
        void reconnect();
      };
      window.addEventListener("online", waitForOnline, { once: true });
    }
  };

  ws = conn;
}

export const socket = {
  connect(id: string) {
    socket.disconnect();
    boardId = id;
    open(id);
  },

  disconnect() {
    clearTimeout(retry);
    if (waitForOnline) window.removeEventListener("online", waitForOnline);
    waitForOnline = null;
    const conn = ws;
    ws = null;
    boardId = null;
    conn?.close();
  },

  emit(event: string, ...args: unknown[]) {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ boardId, event, args }));
    }
  },

  on(event: string, handler: Handler) {
    if (!handlers.has(event)) handlers.set(event, new Set());
    handlers.get(event)!.add(handler);
  },

  off(event: string, handler: Handler) {
    handlers.get(event)?.delete(handler);
  },
};
