import * as React from "react";
import { cn } from "@/lib/utils";

export function Badge({ className, tone = "neutral", ...props }: React.ComponentProps<"span"> & { tone?: "neutral" | "good" | "warn" | "bad" }) {
  const tones = { neutral: "bg-slate-500/15 text-slate-300", good: "bg-emerald-400/15 text-emerald-300", warn: "bg-amber-400/15 text-amber-300", bad: "bg-rose-400/15 text-rose-300" };
  return <span className={cn("inline-flex rounded-full px-2.5 py-1 text-xs font-medium capitalize", tones[tone], className)} {...props} />;
}
