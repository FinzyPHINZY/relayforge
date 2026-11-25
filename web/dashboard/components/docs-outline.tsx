"use client";

import { useEffect, useState } from "react";

const sections = [
  { id: "install", label: "Install" },
  { id: "publish", label: "Publish" },
  { id: "retries", label: "Retries" },
  { id: "errors", label: "Errors" },
  { id: "receive", label: "Receive" },
  { id: "http", label: "HTTP" },
];

export function DocsOutline() {
  const [active, setActive] = useState("install");

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries.filter((entry) => entry.isIntersecting);
        if (visible.length) setActive(visible[0].target.id);
      },
      { rootMargin: "-100px 0px -65% 0px" },
    );
    sections.forEach(({ id }) => {
      const section = document.getElementById(id);
      if (section) observer.observe(section);
    });
    return () => observer.disconnect();
  }, []);

  return <nav className="docs-outline" aria-label="On this page"><div className="landing-container docs-outline-inner">
    <span>ON THIS PAGE</span>
    {sections.map(({ id, label }) => <a key={id} href={`#${id}`} onClick={() => setActive(id)} aria-current={active === id ? "location" : undefined}>{label}</a>)}
  </div></nav>;
}
