export type Panel<T> = { available: true; value: T } | { available: false };

/** settles one overview read: a 403 (missing scope) hides just that panel, anything else still fails the page. */
export function panelResult<T>(result: PromiseSettledResult<T>): Panel<T> {
  if (result.status === "fulfilled") return { available: true, value: result.value };
  const status = (result.reason as { status?: unknown } | null)?.status;
  if (status === 403) return { available: false };
  throw result.reason;
}
