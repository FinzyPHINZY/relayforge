import { Badge } from "@/components/ui/badge";

export function StatusBadge({ status }: { status: string }) {
  const tone = status === "succeeded" || status === "active" ? "good" : status === "retry_wait" || status === "paused" || status === "processing" ? "warn" : status === "exhausted" || status === "cancelled" || status === "disabled" ? "bad" : "neutral";
  return <Badge tone={tone}>{status.replaceAll("_", " ")}</Badge>;
}
