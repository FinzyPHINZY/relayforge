import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = { title: "RelayForge Console", description: "Webhook delivery operations" };

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
