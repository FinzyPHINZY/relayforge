import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva("inline-flex items-center justify-center gap-2 rounded-lg text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#579b4e] disabled:pointer-events-none disabled:opacity-50", {
  variants: {
    variant: {
      default: "bg-[#b7f36d] text-[#14251e] hover:bg-[#c8ff87]",
      secondary: "bg-white/10 text-white hover:bg-white/15",
      outline: "border border-white/15 bg-transparent text-slate-100 hover:bg-white/5",
      ghost: "text-slate-300 hover:bg-white/10 hover:text-white",
      destructive: "bg-rose-500/15 text-rose-300 hover:bg-rose-500/25",
    },
    size: { default: "h-10 px-4 py-2", sm: "h-8 px-3", lg: "h-11 px-5" },
  },
  defaultVariants: { variant: "default", size: "default" },
});

export function Button({ className, variant, size, ...props }: React.ComponentProps<"button"> & VariantProps<typeof buttonVariants>) {
  return <button className={cn(buttonVariants({ variant, size }), className)} {...props} />;
}
