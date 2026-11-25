// only changed fields are sent, and an owner's role is never sent: the form can't
// offer "owner", so sending its select value would silently demote owners.

const editableRoles = ["admin", "operator", "viewer"] as const;
const statuses = ["active", "suspended"] as const;
const knownRoles = ["owner", ...editableRoles];

export type MemberUpdate = { role?: (typeof editableRoles)[number]; status?: (typeof statuses)[number] };

type Original = { role: string; status: string };
type Submitted = { role: string | null; status: string | null };

export function diffMemberUpdate(original: Original, submitted: Submitted): MemberUpdate | null {
  if (!knownRoles.includes(original.role) || !(statuses as readonly string[]).includes(original.status)) return null;
  const update: MemberUpdate = {};
  if (original.role !== "owner" && submitted.role !== null && submitted.role !== original.role) {
    const role = editableRoles.find((r) => r === submitted.role);
    if (!role) return null;
    update.role = role;
  }
  if (submitted.status !== null && submitted.status !== original.status) {
    const status = statuses.find((s) => s === submitted.status);
    if (!status) return null;
    update.status = status;
  }
  return update;
}
