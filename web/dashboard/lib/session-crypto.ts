import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto";

const lifetimeSeconds = 8 * 60 * 60;

export function sealSession(apiKey: string, secret: Buffer, now = Date.now()): string {
  const iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", secret, iv);
  const payload = Buffer.from(JSON.stringify({ key: apiKey, expires: now + lifetimeSeconds * 1000 }));
  const encrypted = Buffer.concat([cipher.update(payload), cipher.final()]);
  return Buffer.concat([iv, cipher.getAuthTag(), encrypted]).toString("base64url");
}

export function openSession(value: string, secret: Buffer, now = Date.now()): string | null {
  try {
    const bytes = Buffer.from(value, "base64url");
    if (bytes.length < 29) return null;
    const decipher = createDecipheriv("aes-256-gcm", secret, bytes.subarray(0, 12));
    decipher.setAuthTag(bytes.subarray(12, 28));
    const payload = Buffer.concat([decipher.update(bytes.subarray(28)), decipher.final()]);
    const parsed: unknown = JSON.parse(payload.toString("utf8"));
    if (!parsed || typeof parsed !== "object") return null;
    const session = parsed as { key?: unknown; expires?: unknown };
    if (typeof session.key !== "string" || !session.key.startsWith("whk_") || typeof session.expires !== "number" || session.expires <= now) return null;
    return session.key;
  } catch {
    return null;
  }
}
