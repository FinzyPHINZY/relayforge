
export type Role = "owner" | "admin" | "operator" | "viewer";

export const allRoles: readonly Role[] = ["owner", "admin", "operator", "viewer"];

export const allScopes: readonly string[] = [
  "events:read", "events:write", "endpoints:read", "endpoints:write",
  "deliveries:read", "deliveries:write", "metrics:read",
  "team:read", "team:write", "keys:read", "keys:write",
];

const viewerScopes = new Set(["events:read", "endpoints:read", "deliveries:read", "metrics:read", "team:read"]);

const rank: Record<Role, number> = { owner: 4, admin: 3, operator: 2, viewer: 1 };

export function isRole(value: string): value is Role {
  return Object.hasOwn(rank, value);
}

function roleAllows(role: string, scope: string): boolean {
  switch (role) {
    case "owner":
    case "admin":
      return true;
    case "operator":
      return scope !== "team:write" && scope !== "keys:write";
    case "viewer":
      return viewerScopes.has(scope);
    default:
      return false;
  }
}

export function allowedRoles(myRole: string): Role[] {
  if (!isRole(myRole)) return [];
  return allRoles.filter((role) => rank[role] <= rank[myRole]);
}

export function allowedScopes(role: string): string[] {
  return allScopes.filter((scope) => roleAllows(role, scope));
}

export function offeredScopes(role: string, myScopes: readonly string[]): string[] {
  const mine = new Set(myScopes);
  return allowedScopes(role).filter((scope) => mine.has(scope));
}

export const expiryChoices = [
  { value: "never", label: "Never", days: null },
  { value: "30", label: "30 days", days: 30 },
  { value: "90", label: "90 days", days: 90 },
  { value: "365", label: "365 days", days: 365 },
] as const;
