"use server";

import { apiJson, ApiError, requireOrgSession } from "@/lib/api";

export type RevealState = { data?: unknown; error?: string };

export async function revealEventPayload(_previous: RevealState, formData: FormData): Promise<RevealState> {
  const id = String(formData.get("eventId") ?? "");
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)) return { error: "Invalid event ID." };
  const session = await requireOrgSession();
  try {
    return await apiJson<RevealState>(session, `/v1/events/${id}/reveal`, { method: "POST" });
  } catch (error) {
    return { error: error instanceof ApiError ? error.message : "Could not reveal the payload." };
  }
}
