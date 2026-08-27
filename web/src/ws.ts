// Reconnecting WebSocket client for /v1/browser (spec.md ss33). The server
// sends a fresh "snapshot" on every new connection, so reconnecting after a
// drop is enough to resynchronize state — no client-side resume logic
// needed.
import type { ClientCmd, ServerMsg } from "./types";

export type WsStatus = "connecting" | "open" | "closed";

const MIN_BACKOFF_MS = 500;
const MAX_BACKOFF_MS = 10_000;

export class BrowserSocket {
  private socket: WebSocket | null = null;
  private backoff = MIN_BACKOFF_MS;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private closedByUser = false;
  private readonly onMessage: (msg: ServerMsg) => void;
  private readonly onStatus: (status: WsStatus) => void;

  constructor(onMessage: (msg: ServerMsg) => void, onStatus: (status: WsStatus) => void) {
    this.onMessage = onMessage;
    this.onStatus = onStatus;
  }

  connect(): void {
    this.closedByUser = false;
    this.open();
  }

  private open(): void {
    this.onStatus("connecting");
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const socket = new WebSocket(`${proto}//${window.location.host}/v1/browser`);
    this.socket = socket;

    socket.onopen = () => {
      this.backoff = MIN_BACKOFF_MS;
      this.onStatus("open");
    };
    socket.onmessage = (ev) => {
      try {
        this.onMessage(JSON.parse(ev.data as string) as ServerMsg);
      } catch {
        // Ignore malformed frames rather than tearing down the connection.
      }
    };
    socket.onclose = () => {
      this.onStatus("closed");
      if (!this.closedByUser) this.scheduleReconnect();
    };
    socket.onerror = () => {
      socket.close();
    };
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF_MS);
      this.open();
    }, this.backoff);
  }

  send(cmd: ClientCmd): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(cmd));
    }
  }

  close(): void {
    this.closedByUser = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.socket?.close();
  }
}
