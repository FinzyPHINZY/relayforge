import { PublicHeader } from "@/components/public-header";
import "./landing.css";

export default function PublicLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <><PublicHeader />{children}</>;
}
