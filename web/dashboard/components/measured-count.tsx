"use client";

import { useEffect, useState } from "react";
import CountUp from "@/components/react-bits-count-up";

export function MeasuredCount() {
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  return mounted ? <CountUp to={10000} separator="," mode="roll" duration={1.4} /> : <span>10,000</span>;
}
