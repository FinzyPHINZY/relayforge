"use client";

import * as React from "react";
import * as SheetPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export const Sheet = SheetPrimitive.Root;
export const SheetTrigger = SheetPrimitive.Trigger;
export const SheetClose = SheetPrimitive.Close;

// "right" is a side panel from 640px up and a bottom sheet below it; "bottom" is always a bottom sheet.
const sides = {
  right: "inset-x-0 bottom-0 max-h-[85vh] rounded-t-2xl border-t sm:inset-y-0 sm:left-auto sm:right-0 sm:h-full sm:max-h-none sm:w-full sm:max-w-md sm:rounded-none sm:border-l sm:border-t-0",
  bottom: "inset-x-0 bottom-0 max-h-[85vh] rounded-t-2xl border-t",
} as const;

export function SheetContent({ className, children, side = "right", ...props }: React.ComponentProps<typeof SheetPrimitive.Content> & { side?: keyof typeof sides }) {
  return <SheetPrimitive.Portal>
    <SheetPrimitive.Overlay className="rf-sheet-overlay fixed inset-0 z-50 bg-[#10231a]/45" />
    {/* the portal renders outside the shell, so the content carries the console palette itself */}
    <SheetPrimitive.Content data-side={side} className={cn("rf-sheet console-theme fixed z-50 flex flex-col border-[#dfe5db] bg-[#f9faf6] text-white shadow-[0_-12px_40px_rgba(16,35,26,0.18)] outline-none", sides[side], className)} {...props}>
      <SheetPrimitive.Close className="absolute right-3 top-3 z-10 inline-flex size-9 items-center justify-center rounded-lg text-slate-400 transition-colors hover:bg-white/5 hover:text-white">
        <X size={18} aria-hidden="true" /><span className="sr-only">Close</span>
      </SheetPrimitive.Close>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 pb-6 pt-5 sm:px-6">{children}</div>
    </SheetPrimitive.Content>
  </SheetPrimitive.Portal>;
}

export function SheetHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("mb-5 grid gap-1.5 pr-10", className)} {...props} />;
}

export function SheetTitle({ className, ...props }: React.ComponentProps<typeof SheetPrimitive.Title>) {
  return <SheetPrimitive.Title className={cn("text-lg font-semibold tracking-tight text-white [overflow-wrap:anywhere]", className)} {...props} />;
}

export function SheetDescription({ className, ...props }: React.ComponentProps<typeof SheetPrimitive.Description>) {
  return <SheetPrimitive.Description className={cn("text-sm text-slate-500", className)} {...props} />;
}
