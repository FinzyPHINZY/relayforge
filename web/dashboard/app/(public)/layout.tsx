import { ViewTransition } from "react";
import { PublicHeader } from "@/components/public-header";
import { PublicSmoothScroll } from "@/components/public-smooth-scroll";
import "./landing.css";

export default function PublicLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <>
      <PublicSmoothScroll />
      <PublicHeader />
      {/* only page-to-page updates animate; entering or leaving the public pages stays instant */}
      <ViewTransition default="none" update="public-page">{children}</ViewTransition>
    </>
  );
}
