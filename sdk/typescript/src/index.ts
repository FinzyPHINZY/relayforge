export interface AcceptedEvent {
  id: string;
  type: string;
  status: string;
  created_at: string;
}

export class RelayForgeError extends Error {
  constructor(public readonly status: number, public readonly code: string, detail: string) {
    super(detail);
    this.name = "RelayForgeError";
  }
}

export interface PublisherOptions {
  fetch?: typeof fetch;
  retries?: number;
}

export class Publisher {
  private readonly base: URL;
  private readonly request: typeof fetch;
  private readonly retries: number;

  constructor(baseURL: string, private readonly apiKey: string, options: PublisherOptions = {}) {
    const parsed = new URL(baseURL);
    if (!(["https:", "http:"].includes(parsed.protocol)) || parsed.username || parsed.password || parsed.search || parsed.hash) {
      throw new Error("Invalid RelayForge URL");
    }
    if (parsed.protocol !== "https:" && !["localhost", "127.0.0.1", "[::1]"].includes(parsed.hostname)) {
      throw new Error("RelayForge URL must use HTTPS outside localhost");
    }
    if (!apiKey.startsWith("whk_") || apiKey.length > 512) throw new Error("Invalid API key");
    const retries = options.retries ?? 2;
    if (!Number.isInteger(retries) || retries < 0 || retries > 10) throw new Error("Invalid retry count");
    parsed.pathname = parsed.pathname.replace(/\/$/, "") + "/";
    this.base = parsed;
    this.request = options.fetch ?? fetch;
    this.retries = retries;
  }

  async publish(type: string, data: unknown, options: { idempotencyKey?: string; signal?: AbortSignal } = {}): Promise<AcceptedEvent> {
    if (!type) throw new Error("Event type required");
    const key = options.idempotencyKey ?? crypto.randomUUID();
    if (!key || key.length > 255) throw new Error("Invalid idempotency key");
    const body = JSON.stringify({ type, data });
    if (body === undefined || new TextEncoder().encode(body).length > 270 * 1024) throw new Error("Event body too large");
    const endpoint = new URL("v1/events", this.base);
    let lastError: unknown;
    for (let attempt = 0; attempt <= this.retries; attempt++) {
      if (attempt > 0) await delay(Math.min(200 * 2 ** (attempt - 1), 2000), options.signal);
      let response: Response;
      try {
        response = await this.request(endpoint, {
          method: "POST",
          headers: { Authorization: `Bearer ${this.apiKey}`, "Content-Type": "application/json", "Idempotency-Key": key },
          body,
          signal: options.signal,
        });
      } catch (error) {
        if (options.signal?.aborted) throw error;
        lastError = error;
        continue;
      }
      if (response.status === 202) {
        const event: unknown = await response.json();
        if (!event || typeof event !== "object" || typeof (event as AcceptedEvent).id !== "string") {
          throw new Error("Invalid acceptance response");
        }
        return event as AcceptedEvent;
      }
      const problem: unknown = await response.json().catch(() => null);
      const code = problem && typeof problem === "object" && "code" in problem && typeof problem.code === "string" ? problem.code : "unknown";
      const detail = problem && typeof problem === "object" && "detail" in problem && typeof problem.detail === "string" ? problem.detail : response.statusText;
      lastError = new RelayForgeError(response.status, code, detail);
      if (![408, 425, 429].includes(response.status) && response.status < 500) throw lastError;
      if (attempt < this.retries) {
        const seconds = Number(response.headers.get("Retry-After"));
        if (Number.isInteger(seconds) && seconds > 0 && seconds <= 30) await delay(seconds * 1000, options.signal);
      }
    }
    throw lastError;
  }
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(signal.reason);
    const timer = setTimeout(() => { signal?.removeEventListener("abort", abort); resolve(); }, ms);
    const abort = () => { clearTimeout(timer); reject(signal?.reason); };
    signal?.addEventListener("abort", abort, { once: true });
  });
}
