const fieldName = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;

export function parseFieldMap(value: string): Record<string, string> {
  if (!value.trim()) return {};
  const parsed: unknown = JSON.parse(value);
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error("Mapping must be a JSON object.");
  const entries = Object.entries(parsed);
  if (entries.length > 32 || entries.some(([destination, source]) => !fieldName.test(destination) || typeof source !== "string" || !fieldName.test(source))) {
    throw new Error("Use at most 32 simple destination and source field names.");
  }
  return Object.fromEntries(entries) as Record<string, string>;
}
