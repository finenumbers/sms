export function formatMoney(v?: string | number | null, currency = "RUB"): string {
  if (v == null || v === "") {
    return "—";
  }
  const n = Number(v);
  if (!Number.isFinite(n)) {
    return String(v);
  }
  return `${n.toLocaleString("ru-RU", { minimumFractionDigits: 2, maximumFractionDigits: 6 })} ${currency}`;
}

export function parsePriceInput(s: string): number | null {
  const t = s.trim().replace(/[\s\u00a0\u202f]/g, "").replace(",", ".");
  if (!t) {
    return null;
  }
  const n = Number(t);
  return Number.isFinite(n) ? n : null;
}

export function formatPriceInput(v?: string | number | null): string {
  if (v == null || v === "") {
    return "";
  }
  const n = typeof v === "number" ? v : parsePriceInput(String(v));
  if (n == null) {
    return String(v);
  }
  return n.toLocaleString("ru-RU", { minimumFractionDigits: 2, maximumFractionDigits: 2 }).replace(/[\u00a0\u202f]/g, " ");
}

export function priceToApi(s: string): string {
  const n = parsePriceInput(s);
  if (n == null) {
    return s.trim();
  }
  return n.toFixed(2);
}

export function formatBalance(v?: string | number | null, currency = "RUB"): string {
  if (v == null || v === "") {
    return "—";
  }
  const n = Number(v);
  if (!Number.isFinite(n)) {
    return String(v);
  }
  const whole = Math.abs(n - Math.round(n)) < 1e-9;
  const amount = n.toLocaleString("ru-RU", {
    minimumFractionDigits: whole ? 0 : 2,
    maximumFractionDigits: 2,
  });
  const unit = currency === "RUB" ? "рублей" : currency;
  return `${amount} ${unit}`;
}
