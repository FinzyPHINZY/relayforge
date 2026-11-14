"use server";

import { randomUUID } from "node:crypto";
import { revalidatePath } from "next/cache";
import { apiJson, ApiError, requireKey } from "@/lib/api";
import { parseFieldMap } from "@/lib/field-map";
import type { Endpoint, Event, TeamMember } from "@/lib/types";

export type ControlState = { error?: string; success?: string; secret?: string; id?: string };

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function failure(error: unknown): ControlState {
  return { error: error instanceof ApiError ? error.message : "Request failed. Please try again." };
}

export async function updateEndpointControls(_previous: ControlState, form: FormData): Promise<ControlState> {
  const key = await requireKey();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  const eventTypes = String(form.get("eventTypes") ?? "").split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);
  if (form.get("exhaustionAlerts") === "on" && !eventTypes.includes("relayforge.delivery.exhausted")) eventTypes.push("relayforge.delivery.exhausted");
  if (eventTypes.length < 1 || eventTypes.length > 100 || eventTypes.some((item) => !/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(item))) return { error: "Add valid event types separated by commas." };
  let fieldMap: Record<string, string>;
  try { fieldMap = parseFieldMap(String(form.get("fieldMap") ?? "")); } catch (error) { return { error: error instanceof Error ? error.message : "Invalid mapping." }; }
  try {
    await apiJson<Endpoint>(key, `/v1/endpoints/${id}`, { method: "PATCH", body: JSON.stringify({ event_types: [...new Set(eventTypes)], field_map: fieldMap }) });
    revalidatePath("/endpoints");
    revalidatePath(`/endpoints/${id}`);
    return { success: "Subscriptions and field mapping saved." };
  } catch (error) { return failure(error); }
}

export async function verifyEndpointControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const key = await requireKey();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  try {
    await apiJson(key, `/v1/endpoints/${id}/verify`, { method: "POST" });
    revalidatePath(`/endpoints/${id}`);
    revalidatePath("/endpoints");
    return { success: "Receiver verified. It can now receive deliveries." };
  } catch (error) { return failure(error); }
}

export async function rotateEndpointControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const key = await requireKey();
  const id = String(form.get("id") ?? "");
  if (!uuid.test(id)) return { error: "Invalid endpoint." };
  try {
    const result = await apiJson<{ signing_secret: string }>(key, `/v1/endpoints/${id}/rotate-secret`, { method: "POST", body: JSON.stringify({ overlap_seconds: 3600 }) });
    revalidatePath(`/endpoints/${id}`);
    return { success: "Secret rotated. The previous secret remains valid for one hour.", secret: result.signing_secret };
  } catch (error) { return failure(error); }
}

export async function publishEventControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const key = await requireKey();
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
    const event = await apiJson<Event>(key, "/v1/events", { method: "POST", headers: { "Idempotency-Key": randomUUID() }, body: JSON.stringify({ type, data, ...(deliverAt ? { deliver_at: deliverAt } : {}) }) });
    revalidatePath("/events");
    revalidatePath("/dashboard");
    return { success: deliverAt ? "Event accepted and scheduled." : "Event accepted for delivery.", id: event.id };
  } catch (error) { return failure(error); }
}

export async function createTeamMemberControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const key = await requireKey();
  const name = String(form.get("name") ?? "").trim();
  const email = String(form.get("email") ?? "").trim();
  const role = String(form.get("role") ?? "");
  if (!name || name.length > 120 || !email || !["admin", "operator", "viewer"].includes(role)) return { error: "Enter a name, email, and valid role." };
  try {
    const member = await apiJson<TeamMember & { api_key: string }>(key, "/v1/team/members", { method: "POST", body: JSON.stringify({ name, email, role }) });
    revalidatePath("/team");
    return { success: "Member created. Copy their API key now; it is shown only once.", secret: member.api_key };
  } catch (error) { return failure(error); }
}

export async function updateTeamMemberControl(_previous: ControlState, form: FormData): Promise<ControlState> {
  const key = await requireKey();
  const id = String(form.get("id") ?? "");
  const role = String(form.get("role") ?? "");
  const status = String(form.get("status") ?? "");
  if (!uuid.test(id) || !["admin", "operator", "viewer"].includes(role) || !["active", "suspended"].includes(status)) return { error: "Invalid member change." };
  try {
    await apiJson(key, `/v1/team/members/${id}`, { method: "PATCH", body: JSON.stringify({ role, status }) });
    revalidatePath("/team");
    return { success: "Member access updated." };
  } catch (error) { return failure(error); }
}
