"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";

export function Code({ label, children }: { label: string; children: string }) {
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(children);
      setCopyState("copied");
      window.setTimeout(() => setCopyState("idle"), 2000);
    } catch {
      setCopyState("failed");
    }
  }

  return <figure className="docs-code">
    <figcaption><span>{label}</span><button type="button" onClick={copyCode} aria-label={copyState === "copied" ? `${label} code copied` : copyState === "failed" ? `Could not copy ${label} code` : `Copy ${label} code`}>
      {copyState === "copied" ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
      {copyState === "copied" ? "Copied" : copyState === "failed" ? "Copy failed" : "Copy"}
    </button></figcaption>
    <pre tabIndex={0} aria-label={`${label} code`}><code>{children}</code></pre>
  </figure>;
}

export function Pair({ go, ts }: { go: string; ts: string }) {
  const [language, setLanguage] = useState<"go" | "ts">("ts");

  return <div className="docs-pair">
    <div className="docs-language-tabs" aria-label="Code language">
      <button type="button" aria-pressed={language === "go"} onClick={() => setLanguage("go")}>Go</button>
      <button type="button" aria-pressed={language === "ts"} onClick={() => setLanguage("ts")}>TypeScript</button>
    </div>
    <div className={`docs-pair-panel${language === "go" ? " is-selected" : ""}`}><Code label="GO">{go}</Code></div>
    <div className={`docs-pair-panel${language === "ts" ? " is-selected" : ""}`}><Code label="TYPESCRIPT">{ts}</Code></div>
  </div>;
}
