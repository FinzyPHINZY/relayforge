import type { Metadata } from "next";
import "./globals.css";
import "./console.css";

export const metadata: Metadata = { title: "RelayForge", description: "Reliable webhook delivery infrastructure" };

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
