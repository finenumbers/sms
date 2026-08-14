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
