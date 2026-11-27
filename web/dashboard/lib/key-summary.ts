import type { ApiKey } from "./types";

export type KeyStatus = { label: "Active" | "Expired" | "Revoked"; tone: "neutral" | "warning" | "danger" };

export function keyStatus(key: ApiKey, now: Date): KeyStatus {
  if (key.revoked_at) return { label: "Revoked", tone: "danger" };
  if (key.expires_at && new Date(key.expires_at).getTime() <= now.getTime()) return { label: "Expired", tone: "warning" };
  return { label: "Active", tone: "neutral" };
}

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

export function lastUsedLabel(key: ApiKey, now: Date): string {
  if (!key.last_used_at) return "Never";
  const elapsed = now.getTime() - new Date(key.last_used_at).getTime();
  if (elapsed < minute) return "Just now";
  if (elapsed < hour) return `${Math.floor(elapsed / minute)}m ago`;
  if (elapsed < day) return `${Math.floor(elapsed / hour)}h ago`;
  if (elapsed < 30 * day) return `${Math.floor(elapsed / day)}d ago`;
  if (elapsed < 365 * day) return `${Math.floor(elapsed / (30 * day))}mo ago`;
  return `${Math.floor(elapsed / (365 * day))}y ago`;
}
