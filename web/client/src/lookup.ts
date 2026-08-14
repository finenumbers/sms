import { ApiError, formatApiError } from "ui";
import type { LookupCheckType } from "./api";

export const jobStatusLabel: Record<string, string> = {
  queued: "в очереди",
  processing: "выполняется",
  completed: "готово",
  completed_with_errors: "готово с ошибками",
  failed: "ошибка",
};

export const itemStatusLabel: Record<string, string> = {
  queued: "в очереди",
  reserved: "резерв",
  pending: "ожидание",
  completed: "готово",
  failed: "ошибка",
};

export const resultLabel: Record<string, string> = {
  reachable: "в сети",
  unreachable: "не в сети",
  pending: "ожидание",
  error: "ошибка",
  unknown: "неизвестно",
};

export const typeLabel: Record<LookupCheckType, string> = {
  hlr: "HLR",
  ping: "Silent SMS",
};

export const sourceLabel: Record<string, string> = {
  single: "один номер",
  bulk: "список",
  api: "API",
};

export function lookupError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "tariff_not_configured" || error.code === "lookup_disabled") {
      return "услуга не подключена";
    }
    if (error.code === "validation" && error.message) {
      return error.message;
    }
  }
  return formatApiError(error);
}

export function parsePhoneList(raw: string): string[] {
  return raw
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function lookupInflight(status?: string): boolean {
  return status === "queued" || status === "processing";
}
