import { type HTMLAttributes } from "react";

interface CardProps extends HTMLAttributes<HTMLDivElement> {}

export function Card({ className = "", children, ...props }: CardProps) {
  return (
    <div
      {...props}
      className={`bg-[var(--surface)] rounded-[var(--radius-lg)] border border-[var(--border)] p-4 ${className}`}
    >
      {children}
    </div>
  );
}
