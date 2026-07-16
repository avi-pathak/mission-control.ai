import { decode, encode, type Envelope, type MessageType } from '@mc/protocol';

export type WSStatus = 'connecting' | 'open' | 'closed';

export interface ReconnectingWSOptions {
  /** URL to connect to. A function is re-evaluated on every (re)connect, so a
   *  fresh auth token is picked up after login/logout. */
  url: string | (() => string);
  onMessage: (env: Envelope) => void;
  onStatus?: (status: WSStatus) => void;
  maxBackoffMs?: number;
}

/**
 * A resilient WebSocket wrapper with exponential backoff reconnection.
 * Framework-agnostic; the Zustand store adapts it to React state.
 */
export class ReconnectingWS {
  private ws: WebSocket | null = null;
  private backoff = 1000;
  private closedByUser = false;
  private readonly maxBackoff: number;

  constructor(private readonly opts: ReconnectingWSOptions) {
    this.maxBackoff = opts.maxBackoffMs ?? 30000;
  }

  connect(): void {
    this.closedByUser = false;
    this.open();
  }

  private open(): void {
    this.opts.onStatus?.('connecting');
    const url = typeof this.opts.url === 'function' ? this.opts.url() : this.opts.url;
    const ws = new WebSocket(url);
    this.ws = ws;

    ws.onopen = () => {
      this.backoff = 1000;
      this.opts.onStatus?.('open');
    };
    ws.onmessage = (ev) => {
      try {
        this.opts.onMessage(decode(ev.data as string));
      } catch {
        /* ignore malformed frames */
      }
    };
    ws.onclose = () => {
      this.opts.onStatus?.('closed');
      if (!this.closedByUser) this.scheduleReconnect();
    };
    ws.onerror = () => ws.close();
  }

  private scheduleReconnect(): void {
    const delay = this.backoff;
    this.backoff = Math.min(this.backoff * 2, this.maxBackoff);
    setTimeout(() => {
      if (!this.closedByUser) this.open();
    }, delay);
  }

  send(type: MessageType, payload: unknown): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(encode(type, payload));
    }
  }

  close(): void {
    this.closedByUser = true;
    this.ws?.close();
  }
}
