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
