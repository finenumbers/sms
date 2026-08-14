import type { ButtonHTMLAttributes, InputHTMLAttributes, LabelHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";
import { formatApiError } from "../lib/api";
import { cn } from "../lib/cn";

export function Button({
  className,
  variant: _variant = "primary",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "ghost" }) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center rounded-md bg-[#FBE95F] px-3 py-2 text-sm font-bold text-black hover:bg-[#F0DC4A] disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={cn(
        "w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-blue-500",
        props.className,
      )}
    />
  );
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={cn(
        "w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-blue-500",
        props.className,
      )}
    />
  );
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      {...props}
      className={cn(
        "w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-blue-500",
        props.className,
      )}
    />
  );
}

export function Label({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn("mb-1 block text-sm font-medium text-zinc-700", className)} {...props} />;
}

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("rounded-lg border border-zinc-200 bg-white p-4 shadow-sm", className)}>{children}</div>;
}

export function Badge({
  children,
  tone = "zinc",
}: {
  children: ReactNode;
  tone?: "zinc" | "blue" | "green" | "amber" | "red";
}) {
  const colors = {
    zinc: "bg-zinc-100 text-zinc-700",
    blue: "bg-blue-50 text-blue-700",
    green: "bg-green-50 text-green-700",
    amber: "bg-amber-50 text-amber-800",
    red: "bg-red-50 text-red-700",
  }[tone];
  return <span className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium", colors)}>{children}</span>;
}

export function Alert({
  children,
  tone = "red",
  className,
}: {
  children: ReactNode;
  tone?: "red" | "green" | "amber";
  className?: string;
}) {
  const colors = {
    red: "border-red-200 bg-red-50 text-red-800",
    green: "border-green-200 bg-green-50 text-green-800",
    amber: "border-amber-200 bg-amber-50 text-amber-900",
  }[tone];
  return <div className={cn("rounded-md border px-3 py-2 text-sm", colors, className)}>{children}</div>;
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="mb-3">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

export function PageHeader({ title, actions }: { title: string; actions?: ReactNode }) {
  return (
    <div className="mb-4 flex items-center justify-between gap-3">
      <h1 className="text-xl font-semibold">{title}</h1>
      {actions}
    </div>
  );
}

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-zinc-200 bg-white">
      <table className="w-full text-left text-sm">{children}</table>
    </div>
  );
}

export function Th({
  children,
  fit,
  fluid,
}: {
  children?: ReactNode;
  fit?: boolean;
  fluid?: boolean;
}) {
  return (
    <th
      className={cn(
        "border-b border-zinc-200 bg-zinc-50 px-3 font-medium text-zinc-600",
        fit && "w-px whitespace-nowrap",
        fluid && "w-full min-w-0",
      )}
    >
      {children}
    </th>
  );
}

export function Td({
  children,
  className,
  fit,
  fluid,
}: {
  children: ReactNode;
  className?: string;
  fit?: boolean;
  fluid?: boolean;
}) {
  return (
    <td
      className={cn(
        "border-b border-zinc-100 px-3",
        fit && "w-px whitespace-nowrap",
        fluid && "w-full min-w-0 truncate",
        className,
      )}
    >
      {children}
    </td>
  );
}

export function statusTone(status: string): "zinc" | "blue" | "green" | "amber" | "red" {
  switch (status) {
    case "active":
    case "delivered":
    case "completed":
    case "done":
      return "green";
    case "queued":
    case "accepted":
    case "sent":
    case "running":
    case "assigned":
    case "processing":
    case "pending":
    case "reserved":
      return "blue";
    case "completed_with_errors":
    case "suspended":
    case "disabled":
    case "cancelled":
    case "revoked":
      return "amber";
    case "failed":
    case "deleted":
    case "dead":
      return "red";
    default:
      return "zinc";
  }
}

export function ErrorBox({ error }: { error: unknown }) {
  if (!error) {
    return null;
  }
  return <Alert>{formatApiError(error)}</Alert>;
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <p className="px-3 py-8 text-center text-sm text-zinc-500">{children}</p>;
}

export function Pager({
  offset,
  limit,
  count,
  onChange,
}: {
  offset: number;
  limit: number;
  count: number;
  onChange: (offset: number) => void;
}) {
  const prev = offset > 0;
  const next = count >= limit;
  if (!prev && !next) {
    return null;
  }
  return (
    <div className="mt-3 flex items-center gap-2 text-sm">
      <Button variant="secondary" type="button" disabled={!prev} onClick={() => onChange(Math.max(0, offset - limit))}>
        Назад
      </Button>
      <span className="text-zinc-500">
        {offset + 1}–{offset + count}
      </span>
      <Button variant="secondary" type="button" disabled={!next} onClick={() => onChange(offset + limit)}>
        Далее
      </Button>
    </div>
  );
}

export type InvalidRow = { line?: number; value?: string; error: string };

export function InvalidList({ rows }: { rows?: InvalidRow[] | null }) {
  if (!rows?.length) {
    return null;
  }
  return (
    <Alert className="mb-3">
      Некорректные строки ({rows.length}):
      <ul className="mt-1 max-h-40 overflow-auto text-xs">
        {rows.slice(0, 50).map((row, i) => (
          <li key={`${row.line ?? i}-${row.value ?? ""}`}>
            {row.line != null ? `стр. ${row.line}: ` : null}
            {row.value ? <code>{row.value}</code> : null} {row.error}
          </li>
        ))}
      </ul>
    </Alert>
  );
}
