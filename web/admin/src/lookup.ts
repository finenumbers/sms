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

export const typeLabel: Record<string, string> = {
  hlr: "HLR",
  ping: "Silent SMS",
};

export const sourceLabel: Record<string, string> = {
  single: "один номер",
  bulk: "список",
  api: "API",
};

export const productLabel: Record<string, string> = {
  sms_domestic: "SMS на номера 7… (как в направлениях платформы)",
  sms_international: "SMS международный",
  hlr: "HLR",
  silent_sms: "Silent SMS",
};

export function priceUnit(product: string): string {
  return product === "hlr" || product === "silent_sms" ? "за проверку" : "за PDU";
}

export function lookupInflight(status?: string): boolean {
  return status === "queued" || status === "processing";
}

export function yn(v?: boolean | null): string {
  if (v == null) {
    return "—";
  }
  return v ? "да" : "нет";
}
