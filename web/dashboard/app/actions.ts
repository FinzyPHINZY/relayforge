"use server";

import { randomUUID } from "node:crypto";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { apiJson, ApiError, requireKey } from "@/lib/api";
import { clearSession, saveSession } from "@/lib/session";
import { parseFieldMap } from "@/lib/field-map";
import type { Endpoint, Metrics } from "@/lib/types";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export async function signIn(formData: FormData) {
  const key = String(formData.get("apiKey") ?? "").trim();
  if (!key.startsWith("whk_") || key.length > 512) redirect("/login?error=invalid");
  try {
    await apiJson<Metrics>(key, "/v1/metrics/summary");
  } catch (error) {
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) redirect("/login?error=invalid");
    redirect("/login?error=unavailable");
  }
  await saveSession(key);
  redirect("/dashboard");
}

export async function signOut() {
  await clearSession();
  redirect("/login");
}

export type CreateEndpointState = { error?: string; secret?: string; endpoint?: Endpoint };

export async function createEndpoint(_previous: CreateEndpointState, formData: FormData): Promise<CreateEndpointState> {
  const key = await requireKey();
  const url = String(formData.get("url") ?? "").trim();
  const description = String(formData.get("description") ?? "").trim();
  const eventTypes = String(formData.get("eventTypes") ?? "").split(/[\s,]+/).map((value) => value.trim()).filter(Boolean);
  if (formData.get("exhaustionAlerts") === "on" && !eventTypes.includes("relayforge.delivery.exhausted")) eventTypes.push("relayforge.delivery.exhausted");
  if (!url || eventTypes.length === 0) return { error: "Add a destination URL and at least one event type." };
  const concurrency = String(formData.get("concurrencyLimit") ?? "2");
  const rate = String(formData.get("rateLimitPerMinute") ?? "0");
  const timeout = String(formData.get("timeoutMs") ?? "10000");
  const maxAttempts = String(formData.get("maxAttempts") ?? "8");
  if (!/^\d+$/.test(concurrency) || Number(concurrency) < 1 || Number(concurrency) > 100 || !/^\d+$/.test(rate) || Number(rate) > 6000) return { error: "Use 1–100 concurrent attempts and 0–6,000 deliveries per minute." };
  if (!/^\d+$/.test(timeout) || Number(timeout) < 1000 || Number(timeout) > 30000 || !/^\d+$/.test(maxAttempts) || Number(maxAttempts) < 1 || Number(maxAttempts) > 8) return { error: "Use a 1,000–30,000 ms timeout and 1–8 total attempts." };
  let fieldMap: Record<string, string>;
  try { fieldMap = parseFieldMap(String(formData.get("fieldMap") ?? "")); } catch (error) { return { error: error instanceof Error ? error.message : "Invalid mapping." }; }
  try {
    const response = await apiJson<{ endpoint: Endpoint; signing_secret: string }>(key, "/v1/endpoints", {
      method: "POST",
      body: JSON.stringify({ url, description, event_types: [...new Set(eventTypes)], verification_required: formData.get("verificationRequired") === "on", field_map: fieldMap, concurrency_limit: Number(concurrency), rate_limit_per_minute: Number(rate), timeout_ms: Number(timeout), max_attempts: Number(maxAttempts) }),
    });
    revalidatePath("/endpoints");
    return { endpoint: response.endpoint, secret: response.signing_secret };
  } catch (error) {
    return { error: error instanceof ApiError ? error.message : "Could not create the endpoint." };
  }
}

export async function setEndpointStatus(formData: FormData) {
  const key = await requireKey();
  const id = String(formData.get("id") ?? "");
  const status = String(formData.get("status") ?? "");
  if (!uuid.test(id) || (status !== "active" && status !== "paused")) redirect("/endpoints?error=invalid");
  try {
    await apiJson<Endpoint>(key, `/v1/endpoints/${id}`, { method: "PATCH", body: JSON.stringify({ status }) });
  } catch {
    redirect("/endpoints?error=update");
  }
  revalidatePath("/endpoints");
  redirect("/endpoints");
}

export async function replayDelivery(formData: FormData) {
  const key = await requireKey();
  const id = String(formData.get("id") ?? "");
  if (!uuid.test(id)) redirect("/deliveries?error=invalid");
  let replay: { id: string };
  try {
    replay = await apiJson<{ id: string }>(key, `/v1/deliveries/${id}/replay`, {
      method: "POST",
      headers: { "Idempotency-Key": randomUUID() },
    });
  } catch {
    redirect(`/deliveries/${id}?error=replay`);
  }
  revalidatePath("/deliveries");
  redirect(`/deliveries/${replay.id}`);
}

export async function reviewDelivery(formData: FormData) {
  const key = await requireKey();
  const id = String(formData.get("id") ?? "");
  const status = String(formData.get("status") ?? "");
  const note = String(formData.get("note") ?? "").trim();
  if (!uuid.test(id) || (status !== "investigating" && status !== "resolved") || note.length > 500 || (status === "resolved" && !note)) {
    redirect(`/deliveries/${encodeURIComponent(id)}?error=review`);
  }
  try {
    await apiJson(key, `/v1/deliveries/${id}/review`, {
      method: "PATCH",
      body: JSON.stringify({ status, note }),
    });
  } catch {
    redirect(`/deliveries/${id}?error=review`);
  }
  revalidatePath("/dead-letters");
  revalidatePath("/deliveries");
  revalidatePath(`/deliveries/${id}`);
  redirect(`/deliveries/${id}`);
}
