import { apiFetch } from "./apiFetch";

type Handler = (...args: never[]) => void;

const handlers = new Map<string, Set<Handler>>();
let ws: WebSocket | null = null;
let boardId: string | null = null;
let retry: ReturnType<typeof setTimeout> | undefined;

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
    retry = setTimeout(async () => {
      await apiFetch("/api/me");
      if (ws === conn) open(id);
    }, 3000);
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
