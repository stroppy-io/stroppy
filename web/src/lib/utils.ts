import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function scrollbarCn(...inputs: ClassValue[]) {
    return cn(
        ...inputs,
        "[scrollbar-width:thin] [scrollbar-color:var(--primary)_transparent]" +
        " [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:rounded-full" +
        " [&::-webkit-scrollbar-track]:bg-muted/30 [&::-webkit-scrollbar-thumb]:rounded-full" +
        " [&::-webkit-scrollbar-thumb]:bg-primary-foreground/40" +
        " hover:[&::-webkit-scrollbar-thumb]:bg-primary-foreground/60"
    )
}
