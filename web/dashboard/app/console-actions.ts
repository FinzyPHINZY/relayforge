"use server";

import { randomUUID } from "node:crypto";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { apiJson, ApiError, requireOrgSession, requireSession } from "@/lib/api";
import { parseFieldMap } from "@/lib/field-map";
import { allScopes, expiryChoices, isRole } from "@/lib/key-form";
import { clearSession } from "@/lib/session";
import { uuidPattern } from "@/lib/session-value";
import type { AccountSession, ApiKey, Endpoint, Event, TeamMember } from "@/lib/types";

export type ControlState = { error?: string; success?: string; secret?: string; id?: string };

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function failure(error: unknown): ControlState {
  return { error: error instanceof ApiError ? error.message : "Request failed. Please try again." };
}

export async function updateEndpointControls(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  const eventTypes = String(form.get("eventTypes") ?? "").split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);
  if (form.get("exhaustionAlerts") === "on" && !eventTypes.includes("relayforge.delivery.exhausted")) eventTypes.push("relayforge.delivery.exhausted");
  if (eventTypes.length < 1 || eventTypes.length > 100 || eventTypes.some((item) => !/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(item))) return { error: "Add valid event types separated by commas." };
  let fieldMap: Record<string, string>;
  try { fieldMap = parseFieldMap(String(form.get("fieldMap") ?? "")); } catch (error) { return { error: error instanceof Error ? error.message : "Invalid mapping." }; }
  try {
    await apiJson<Endpoint>(session, `/v1/endpoints/${id}`, { method: "PATCH", body: JSON.stringify({ event_types: [...new Set(eventTypes)], field_map: fieldMap }) });
    revalidatePath("/endpoints");
    revalidatePath(`/endpoints/${id}`);
    return { success: "Subscriptions and field mapping saved." };
  } catch (error) { return failure(error); }
}

export async function updateEndpointDeliveryPolicy(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const id = String(form.get("id") ?? "");
  const concurrency = String(form.get("concurrencyLimit") ?? "");
  const rate = String(form.get("rateLimitPerMinute") ?? "");
  const timeout = String(form.get("timeoutMs") ?? "");
  const maxAttempts = String(form.get("maxAttempts") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  if (!/^\d+$/.test(concurrency) || Number(concurrency) < 1 || Number(concurrency) > 100 || !/^\d+$/.test(rate) || Number(rate) > 6000) return { error: "Use 1–100 concurrent attempts and 0–6,000 deliveries per minute." };
  if (!/^\d+$/.test(timeout) || Number(timeout) < 1000 || Number(timeout) > 30000 || !/^\d+$/.test(maxAttempts) || Number(maxAttempts) < 1 || Number(maxAttempts) > 8) return { error: "Use a 1,000–30,000 ms timeout and 1–8 total attempts." };
  try {
    await apiJson<Endpoint>(session, `/v1/endpoints/${id}`, { method: "PATCH", body: JSON.stringify({ concurrency_limit: Number(concurrency), rate_limit_per_minute: Number(rate), timeout_ms: Number(timeout), max_attempts: Number(maxAttempts) }) });
    revalidatePath("/endpoints");
    revalidatePath(`/endpoints/${id}`);
    return { success: "Delivery policy saved. New delivery runs use the timeout and retry limit; the current rate slot is retained." };
  } catch (error) { return failure(error); }
}

export async function verifyEndpointControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  try {
    await apiJson(session, `/v1/endpoints/${id}/verify`, { method: "POST" });
    revalidatePath(`/endpoints/${id}`);
    revalidatePath("/endpoints");
    return { success: "Receiver verified. It can now receive deliveries." };
  } catch (error) { return failure(error); }
}

export async function rotateEndpointControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  try {
    const result = await apiJson<{ signing_secret: string }>(session, `/v1/endpoints/${id}/rotate-secret`, { method: "POST", body: JSON.stringify({ overlap_seconds: 3600 }) });
    revalidatePath(`/endpoints/${id}`);
    return { success: "Secret rotated. The previous secret remains valid for one hour.", secret: result.signing_secret };
  } catch (error) { return failure(error); }
}

export async function publishEventControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const type = String(form.get("type") ?? "").trim();
  if (!/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(type) || type.startsWith("relayforge.") || type.length > 128) return { error: "Use an event type like order.created." };
  let data: unknown;
  try { data = JSON.parse(String(form.get("data") ?? "")); } catch { return { error: "Data must be valid JSON." }; }
  if (data === null) return { error: "Data cannot be null." };
  const schedule = String(form.get("deliverAt") ?? "").trim();
  let deliverAt: string | undefined;
  if (schedule) {
    const date = new Date(schedule);
    if (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now() || date.getTime() > Date.now() + 30 * 24 * 60 * 60 * 1000) return { error: "Choose a time within the next 30 days." };
    deliverAt = date.toISOString();
  }
  try {
    const event = await apiJson<Event>(session, "/v1/events", { method: "POST", headers: { "Idempotency-Key": randomUUID() }, body: JSON.stringify({ type, data, ...(deliverAt ? { deliver_at: deliverAt } : {}) }) });
    revalidatePath("/events");
    revalidatePath("/dashboard");
    return { success: deliverAt ? "Event accepted and scheduled." : "Event accepted for delivery.", id: event.id };
  } catch (error) { return failure(error); }
}

export async function createTeamMemberControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const name = String(form.get("name") ?? "").trim();
  const email = String(form.get("email") ?? "").trim();
  const role = String(form.get("role") ?? "");
  if (!name || name.length > 120 || !email || !["admin", "operator", "viewer"].includes(role)) return { error: "Enter a name, email, and valid role." };
  try {
    const member = await apiJson<TeamMember & { api_key: string }>(session, "/v1/team/members", { method: "POST", body: JSON.stringify({ name, email, role }) });
    revalidatePath("/team");
    return { success: "Member created. Copy their API key now; it is shown only once.", secret: member.api_key };
  } catch (error) { return failure(error); }
}

export async function updateTeamMemberControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const id = String(form.get("id") ?? "");
  const role = String(form.get("role") ?? "");
  const status = String(form.get("status") ?? "");
  if (!uuid.test(id) || !["admin", "operator", "viewer"].includes(role) || !["active", "suspended"].includes(status)) return { error: "Invalid member change." };
  try {
    await apiJson(session, `/v1/team/members/${id}`, { method: "PATCH", body: JSON.stringify({ role, status }) });
    revalidatePath("/team");
    return { success: "Member access updated." };
  } catch (error) { return failure(error); }
}

export async function createKeyControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const name = String(form.get("name") ?? "").trim();
  const role = String(form.get("role") ?? "");
  const scopes = [...new Set(form.getAll("scopes").map(String))];
  const expiry = expiryChoices.find((choice) => choice.value === String(form.get("expiry") ?? ""));
  if (name.length < 1 || [...name].length > 80) return { error: "Give the key a name of 1 to 80 characters." };
  if (!isRole(role) || !expiry) return { error: "Choose a valid role and expiry." };
  if (scopes.length === 0) return { error: "Choose at least one scope." };
  if (scopes.some((scope) => !allScopes.includes(scope))) return { error: "Choose scopes from the list." };
  try {
    const created = await apiJson<ApiKey & { key: string }>(session, "/v1/keys", { method: "POST", body: JSON.stringify({ name, role, scopes, expires_in_days: expiry.days }) });
    revalidatePath("/keys");
    return { success: `Key "${created.name}" created.`, secret: created.key, id: created.id };
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    if (error instanceof ApiError && error.code === "scope_exceeds_role") return { error: "That role or scope is above your own access." };
    if (error instanceof ApiError && error.code === "invalid_scopes") return { error: "Choose at least one scope the selected role allows." };
    if (error instanceof ApiError && error.code === "invalid_key") return { error: "Give the key a name of 1 to 80 characters and choose a valid role and expiry." };
    if (error instanceof ApiError && error.status === 403) return { error: "You don't have permission to create keys." };
    return failure(error);
  }
}

export async function revokeKeyControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const session = await requireOrgSession();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid key." };
  try {
    await apiJson(session, `/v1/keys/${id}`, { method: "DELETE" });
    revalidatePath("/keys");
    return { success: "Key revoked." };
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    if (error instanceof ApiError && error.code === "cannot_revoke_self") return { error: "You can't revoke the key you're signed in with." };
    if (error instanceof ApiError && error.status === 403) return { error: "You don't have permission to revoke this key." };
    if (error instanceof ApiError && error.status === 404) return { error: "This key no longer exists." };
    return failure(error);
  }
}

// Account calls never carry an org header: they belong to the person, not a workspace.
async function requireUserToken(): Promise<string> {
  const session = await requireSession();
  if (session.kind !== "user") redirect("/dashboard");
  return session.token;
}

export async function revokeSessionControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const token = await requireUserToken();
  const id = String(form.get("id") ?? "");
  if (!uuidPattern.test(id)) return { error: "Invalid session." };
  try {
    await apiJson({ kind: "user", token, org: null }, `/v1/account/sessions/${id}`, { method: "DELETE" });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    if (!(error instanceof ApiError && error.status === 404)) return failure(error);
  }
  revalidatePath("/account");
  return { success: "Session signed out." };
}

// Revokes every other session first, then this one, so a failure part-way
// leaves the user signed in here and able to retry.
export async function signOutEverywhereControl(_previous: ControlState): Promise<ControlState> {
  const token = await requireUserToken();
  const session = { kind: "user", token, org: null } as const;
  try {
    const { data } = await apiJson<{ data: AccountSession[] }>(session, "/v1/account/sessions");
    const others = data.filter((item) => !item.current && uuidPattern.test(item.id));
    const results = await Promise.allSettled(others.map((item) => apiJson(session, `/v1/account/sessions/${item.id}`, { method: "DELETE" })));
    const failed = results.some((result) => result.status === "rejected" && !(result.reason instanceof ApiError && result.reason.status === 404));
    if (failed) {
      revalidatePath("/account");
      return { error: "Some sessions could not be signed out. Try again." };
    }
    await apiJson(session, "/v1/account/sign-out", { method: "POST" });
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 401)) return failure(error);
  }
  await clearSession();
  redirect("/login");
}
