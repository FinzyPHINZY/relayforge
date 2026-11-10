"use server";

import { randomUUID } from "node:crypto";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { apiJson, ApiError, requireKey } from "@/lib/api";
import { clearSession, saveSession } from "@/lib/session";
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
  redirect("/");
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
  if (!url || eventTypes.length === 0) return { error: "Add a destination URL and at least one event type." };
  try {
    const response = await apiJson<{ endpoint: Endpoint; signing_secret: string }>(key, "/v1/endpoints", {
      method: "POST",
      body: JSON.stringify({ url, description, event_types: eventTypes }),
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
