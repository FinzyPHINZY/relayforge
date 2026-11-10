import { Badge } from "@/components/ui/badge";

export function StatusBadge({ status }: { status: string }) {
  const tone = status === "succeeded" || status === "active" || status === "closed" || status === "resolved" ? "good" : status === "retry_wait" || status === "paused" || status === "processing" || status === "half_open" || status === "investigating" ? "warn" : status === "exhausted" || status === "cancelled" || status === "disabled" || status === "open" ? "bad" : "neutral";
  return <Badge tone={tone}>{status.replaceAll("_", " ")}</Badge>;
}
