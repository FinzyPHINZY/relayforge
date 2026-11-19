import type { Metadata } from "next";
import Link from "next/link";
import { ArrowLeft, ArrowRight, ArrowUpRight, CircleAlert, KeyRound, Webhook } from "lucide-react";
import "../landing.css";
import "../how-it-works/works.css";
import "./docs.css";

export const metadata: Metadata = {
  title: "SDK and API guide | RelayForge",
  description: "Install the RelayForge Go or TypeScript publisher SDK, publish and schedule events with idempotency keys, handle errors, and verify signed webhooks on your receiver.",
};

const installGo = `go get github.com/FinzyPHINZY/relayforge/sdk/go/relayforge

import "github.com/FinzyPHINZY/relayforge/sdk/go/relayforge"`;

const installTs = `npm install @finzyphinzy/relayforge

import { Publisher } from "@finzyphinzy/relayforge";`;

const publishGo = `client, err := relayforge.New(
	os.Getenv("RELAYFORGE_URL"),     // e.g. http://localhost:8080
	os.Getenv("RELAYFORGE_API_KEY"), // whk_..., needs events:write
)
if err != nil {
	log.Fatal(err)
}

event, err := client.Publish(ctx, "order.created",
	map[string]any{"order_id": "ord_81f2", "total": 4200},
	"order.created:ord_81f2", // idempotency key
)
if err != nil {
	return err
}
log.Printf("accepted %s (%s)", event.ID, event.Status)`;

const publishTs = `import { Publisher } from "@finzyphinzy/relayforge";

const relayforge = new Publisher(
  process.env.RELAYFORGE_URL!,     // e.g. http://localhost:8080
  process.env.RELAYFORGE_API_KEY!, // whk_..., needs events:write
);

const event = await relayforge.publish(
  "order.created",
  { order_id: "ord_81f2", total: 4200 },
  { idempotencyKey: "order.created:ord_81f2" },
);
console.log(\`accepted \${event.id} (\${event.status})\`);`;

const scheduleGo = `deliverAt := time.Now().Add(24 * time.Hour)

event, err := client.PublishAt(ctx, "trial.ending",
	map[string]any{"account_id": "acc_19"},
	deliverAt,
	"trial.ending:acc_19",
)`;

const scheduleTs = `const deliverAt = new Date(Date.now() + 24 * 60 * 60 * 1000);

const event = await relayforge.publish(
  "trial.ending",
  { account_id: "acc_19" },
  { idempotencyKey: "trial.ending:acc_19", deliverAt },
);`;

const optionsGo = `client, err := relayforge.New(url, key,
	relayforge.WithRetries(4),             // 0–10, default 2
	relayforge.WithHTTPClient(&http.Client{ // default: 10s timeout
		Timeout: 5 * time.Second,
	}),
)

// Cancel the publish, including its retry waits
ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
defer cancel()`;

const optionsTs = `const relayforge = new Publisher(url, key, {
  retries: 4,          // 0–10, default 2
  fetch: customFetch,  // default: global fetch
});

// Cancel the publish, including its retry waits
await relayforge.publish("order.created", data, {
  idempotencyKey,
  signal: AbortSignal.timeout(15_000),
});`;

const errorsGo = `var apiErr *relayforge.APIError
if errors.As(err, &apiErr) {
	switch apiErr.Code {
	case "idempotency_conflict":
		// this key was already used for a different event
	case "quota_exceeded":
		// monthly event quota reached
	}
	log.Printf("HTTP %d %s: %s",
		apiErr.StatusCode, apiErr.Code, apiErr.Message)
}`;

const errorsTs = `import { RelayForgeError } from "@finzyphinzy/relayforge";

try {
  await relayforge.publish(type, data, { idempotencyKey });
} catch (error) {
  if (error instanceof RelayForgeError) {
    console.error(error.status, error.code, error.message);
  } else {
    throw error; // network failure after every retry
  }
}`;

const verifyTs = `import { createHmac, timingSafeEqual } from "node:crypto";

export function verifyWebhook(
  secret: string,     // whsec_... shown once when the endpoint is created
  rawBody: Buffer,    // the exact bytes received, before JSON parsing
  timestamp: string,  // webhook-timestamp header
  signature: string,  // webhook-signature header
  toleranceSeconds = 300,
): boolean {
  if (!/^\\d+$/.test(timestamp)) return false;
  const age = Math.abs(Date.now() / 1000 - Number(timestamp));
  if (age > toleranceSeconds) return false;

  const expected = "v1," + createHmac("sha256", secret)
    .update(\`\${timestamp}.\`)
    .update(rawBody)
    .digest("base64");

  const a = Buffer.from(signature);
  const b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}`;

const verifyGo = `func verifyWebhook(secret string, r *http.Request, body []byte) bool {
	ts := r.Header.Get("webhook-timestamp")
	sent, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if age := time.Since(time.Unix(sent, 0)); age > 5*time.Minute || age < -5*time.Minute {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(r.Header.Get("webhook-signature")), []byte(want))
}`;

const handlerTs = `app.post("/webhooks/relayforge", express.raw({ type: "application/json" }), async (req, res) => {
  const ok = verifyWebhook(
    process.env.RELAYFORGE_SIGNING_SECRET!,
    req.body,
    req.get("webhook-timestamp") ?? "",
    req.get("webhook-signature") ?? "",
  );
  if (!ok) return res.sendStatus(401);

  const deliveryId = req.get("webhook-id")!;
  const event = JSON.parse(req.body.toString("utf8"));

  // Record deliveryId in the same transaction as your side effect.
  // A unique constraint turns a duplicate delivery into a no-op.
  await db.transaction(async (tx) => {
    const fresh = await tx.insertIgnoreDuplicate("processed_webhooks", { id: deliveryId });
    if (fresh) await handle(tx, event);
  });

  res.sendStatus(204);
});`;

const payload = `POST /webhooks/relayforge HTTP/1.1
Content-Type: application/json
webhook-id: dlv_...
webhook-timestamp: 1763560800
webhook-signature: v1,K5o0...base64...=
webhook-attempt: 2

{
  "id": "evt_...",
  "type": "order.created",
  "created_at": "2026-11-19T12:00:00.123456Z",
  "data": { "order_id": "ord_81f2", "total": 4200 }
}`;

const curl = `curl -i "$RELAYFORGE_URL/v1/events" \\
  -H "Authorization: Bearer $RELAYFORGE_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: order.created:ord_81f2" \\
  -d '{"type":"order.created","data":{"order_id":"ord_81f2","total":4200}}'

# Optional: add "deliver_at":"2026-11-20T09:00:00Z" to schedule it.

HTTP/1.1 202 Accepted
{"id":"evt_...","type":"order.created","status":"accepted","created_at":"..."}`;

const errorRows = [
  { status: "202", code: "—", meaning: "Accepted. The same key and body sent again returns the original event.", retry: "—" },
  { status: "401", code: "unauthorized", meaning: "Missing, revoked, or expired API key.", retry: "No" },
  { status: "403", code: "forbidden", meaning: "The key lacks the events:write scope.", retry: "No" },
  { status: "409", code: "idempotency_conflict", meaning: "The key was already used for a different type, body, or deliver_at.", retry: "No" },
  { status: "413", code: "payload_too_large", meaning: "Event data exceeds 256 KiB.", retry: "No" },
  { status: "422", code: "invalid_event", meaning: "Bad event type, or data that is not valid JSON.", retry: "No" },
  { status: "422", code: "invalid_schedule", meaning: "deliver_at is in the past or more than 30 days away.", retry: "No" },
  { status: "429", code: "rate_limited", meaning: "Too many API requests. Honors Retry-After.", retry: "Yes" },
  { status: "429", code: "quota_exceeded", meaning: "Monthly event quota reached. Retrying will not help until the quota resets.", retry: "Yes" },
  { status: "503", code: "storage_unavailable", meaning: "The API could not commit the event.", retry: "Yes" },
];

const headerRows = [
  { name: "webhook-id", body: "Stable delivery ID. It is the same on every retry of a delivery, so use it as your deduplication key." },
  { name: "webhook-timestamp", body: "Unix seconds when this attempt was signed. Reject anything outside your tolerance, such as five minutes." },
  { name: "webhook-signature", body: "v1, followed by base64(HMAC-SHA256(secret, timestamp + \".\" + raw body))." },
  { name: "webhook-attempt", body: "Attempt number, starting at 1. For logging only; do not use it in signature checks." },
];

function Code({ label, children }: { label: string; children: string }) {
  return <figure className="docs-code"><figcaption>{label}</figcaption><pre><code>{children}</code></pre></figure>;
}

function Pair({ go, ts }: { go: string; ts: string }) {
  return <div className="docs-pair"><Code label="GO">{go}</Code><Code label="TYPESCRIPT">{ts}</Code></div>;
}

export default function DocsPage() {
  return <div className="landing workings docs">
    <header className="landing-header"><div className="landing-container landing-header-inner">
      <Link href="/" className="landing-brand" aria-label="RelayForge home"><span className="landing-brand-mark"><Webhook size={21} aria-hidden="true" /></span><span>relayforge<span className="landing-brand-period">.</span></span></Link>
      <nav className="workings-nav" aria-label="Page navigation"><a href="#install">Install</a><a href="#publish">Publish</a><a href="#retries">Retries</a><a href="#errors">Errors</a><a href="#receive">Receive</a></nav>
      <Link href="/dashboard" className="landing-header-cta">Open console <ArrowUpRight size={16} aria-hidden="true" /></Link>
    </div></header>

    <main>
      <section className="landing-container docs-hero" aria-labelledby="docs-title">
        <Link href="/" className="workings-back"><ArrowLeft size={15} aria-hidden="true" /> Back to RelayForge</Link>
        <span className="landing-kicker">SDK AND API GUIDE</span>
        <h1 id="docs-title">Publish from your server.<br /><span>Verify on your receiver.</span></h1>
        <p>RelayForge ships small publisher clients for Go and TypeScript. They send events with a stable idempotency key and retry transient failures for you. This page covers installing them, publishing and scheduling events, handling errors, and verifying the signed webhooks your endpoints receive.</p>
        <div className="docs-callout"><KeyRound size={19} aria-hidden="true" /><p><strong>Before you start:</strong> create an API key with the <code>events:write</code> scope, and register at least one endpoint subscribed to your event type. Keep the key in server code only. Never bundle it into browser JavaScript.</p></div>
      </section>

      <section id="install" className="workings-section docs-section docs-tint" aria-labelledby="install-title"><div className="landing-container">
        <div className="workings-section-heading"><span className="landing-kicker">01 / INSTALL</span><h2 id="install-title">Add a client to your service.</h2><p>The SDKs live in the <a href="https://github.com/FinzyPHINZY/relayforge/tree/main/sdk" target="_blank" rel="noopener noreferrer">repository&apos;s sdk folder</a>. The Go module needs Go 1.22 or newer and uses only the standard library. The TypeScript package has no runtime dependencies. The TypeScript client needs Node.js 20 or newer for global <code>fetch</code> and <code>crypto.randomUUID</code>.</p></div>
        <Pair go={installGo} ts={installTs} />
      </div></section>

      <section id="publish" className="workings-section docs-section" aria-labelledby="publish-title"><div className="landing-container">
        <div className="workings-section-heading"><span className="landing-kicker">02 / PUBLISH</span><h2 id="publish-title">Send an event.</h2><p>Each event has a type and JSON data. Types are lowercase and dotted, such as <code>order.created</code>, and cannot start with <code>relayforge.</code>. Data can be up to 256 KiB. A successful call returns after the event and its outbox row commit together, so an accepted event will not be lost.</p></div>
        <Pair go={publishGo} ts={publishTs} />
        <div className="workings-contract"><CircleAlert size={19} aria-hidden="true" /><p><strong>Choose your own idempotency key.</strong> If you leave it out, the client generates one per call. That covers its own retries, but not your process restarting and calling again. Derive the key from the thing that happened, such as <code>order.created:ord_81f2</code>, so any retry of the same business action reuses it.</p></div>

        <div id="schedule" className="docs-sub">
          <h3>Schedule for later</h3>
          <p>Pass a delivery time up to 30 days ahead. The event is accepted now and dispatched when it is due. If you retry after a restart, keep both the same key and the same time; a different time with the same key is a conflict.</p>
          <Pair go={scheduleGo} ts={scheduleTs} />
        </div>
      </div></section>

      <section id="retries" className="workings-section docs-section docs-tint" aria-labelledby="retries-title"><div className="landing-container">
        <div className="workings-section-heading"><span className="landing-kicker">03 / RETRIES</span><h2 id="retries-title">What the client retries for you.</h2><p>Both clients send the same idempotency key on every attempt, so a retry after a lost response can never create a second event.</p></div>
        <ul className="docs-facts">
          <li><strong>2 retries</strong><span>by default, so up to three requests. Configure 0–10.</span></li>
          <li><strong>200 ms → 2 s</strong><span>exponential backoff between attempts, capped at two seconds.</span></li>
          <li><strong>Retry-After</strong><span>honored for values up to 30 seconds.</span></li>
          <li><strong>Retried</strong><span>network errors, 408, 425, 429, and every 5xx. Other 4xx responses fail immediately.</span></li>
        </ul>
        <Pair go={optionsGo} ts={optionsTs} />
      </div></section>

      <section id="errors" className="workings-section docs-section" aria-labelledby="errors-title"><div className="landing-container">
        <div className="workings-section-heading"><span className="landing-kicker">04 / ERRORS</span><h2 id="errors-title">Handle what comes back.</h2><p>API failures surface as <code>*relayforge.APIError</code> in Go and <code>RelayForgeError</code> in TypeScript, carrying the HTTP status and a machine-readable code. Network failures that outlast every retry surface as the underlying error.</p></div>
        <div className="docs-table-wrap"><table className="docs-table">
          <thead><tr><th>Status</th><th>Code</th><th>Meaning</th><th>Client retries</th></tr></thead>
          <tbody>{errorRows.map((row) => <tr key={row.status + row.code}><td><code>{row.status}</code></td><td><code>{row.code}</code></td><td>{row.meaning}</td><td>{row.retry}</td></tr>)}</tbody>
        </table></div>
        <Pair go={errorsGo} ts={errorsTs} />
      </div></section>

      <section id="receive" className="workings-section docs-section docs-dark" aria-labelledby="receive-title"><div className="landing-container">
        <div className="workings-section-heading"><span className="landing-kicker">05 / RECEIVE</span><h2 id="receive-title">Verify every webhook.<br /><em>Then deduplicate it.</em></h2><p>Delivery is at least once. A worker can crash after your endpoint returns 2xx but before it records that response, and the next worker sends the same delivery again. Check the signature, reject stale timestamps, and treat <code>webhook-id</code> as your deduplication key.</p></div>
        <div className="docs-receive-grid">
          <Code label="WHAT YOUR ENDPOINT RECEIVES">{payload}</Code>
          <dl className="docs-headers">{headerRows.map(({ name, body }) => <div key={name}><dt><code>{name}</code></dt><dd>{body}</dd></div>)}</dl>
        </div>
        <Pair go={verifyGo} ts={verifyTs} />
        <Code label="TYPESCRIPT · EXPRESS HANDLER">{handlerTs}</Code>
        <div className="docs-notes">
          <p><strong>Sign the raw bytes.</strong> Compute the HMAC over the body exactly as received. Parsing and re-serializing JSON changes the bytes and breaks the signature.</p>
          <p><strong>Respond quickly.</strong> Any 2xx acknowledges the attempt. Do slow work after recording the delivery ID, or the attempt may time out and be retried.</p>
          <p><strong>Rotating secrets.</strong> Rotation returns a new secret and keeps the previous one valid until <code>previous_secret_valid_until</code>. During that window, accept a signature that matches either secret.</p>
        </div>
      </div></section>

      <section id="http" className="workings-section docs-section" aria-labelledby="http-title"><div className="landing-container">
        <div className="workings-section-heading"><span className="landing-kicker">NO SDK? NO PROBLEM</span><h2 id="http-title">The raw HTTP call.</h2><p>The SDKs wrap a single endpoint. From any other language, send a <code>POST /v1/events</code> with a bearer key and an <code>Idempotency-Key</code> header, and retry with the same key on network errors, 408, 425, 429, and 5xx.</p></div>
        <Code label="SHELL">{curl}</Code>
      </div></section>

      <section className="landing-container workings-end"><div><span className="landing-kicker">NEXT</span><h2>See where your event goes next.</h2><p>Follow the outbox, the retry schedule, and the crash window behind at-least-once delivery.</p></div><Link href="/how-it-works" className="landing-button landing-button-primary">How RelayForge works <ArrowRight size={17} aria-hidden="true" /></Link></section>
    </main>

    <footer className="landing-footer"><div className="landing-container landing-footer-inner"><Link href="/" className="landing-brand" aria-label="RelayForge home"><span className="landing-brand-mark"><Webhook size={21} aria-hidden="true" /></span><span>relayforge<span className="landing-brand-period">.</span></span></Link><span>Reliable delivery, made visible.</span><Link href="/" className="workings-footer-link">Back home <ArrowRight size={14} aria-hidden="true" /></Link></div></footer>
  </div>;
}
