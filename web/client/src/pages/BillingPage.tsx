import { useQuery } from "@tanstack/react-query";
import { Card, EmptyState, ErrorBox, PAGE_SIZE, PageHeader, Pager, Table, Td, Th, withPage, formatMoney } from "ui";
import { useState } from "react";
import { api, type Balance, type BillingStats, type ClientTariff, type LedgerRow } from "../api";
import { typeLabel } from "../lookup";

const productLabel: Record<string, string> = {
  sms_domestic: "SMS на номера 7…",
  sms_international: "SMS международный",
  hlr: "HLR",
  silent_sms: "Silent SMS",
};

function priceUnit(product: string): string {
  return product === "hlr" || product === "silent_sms" ? "за проверку" : "за PDU";
}

export function BillingPage() {
  const [offset, setOffset] = useState(0);
  const bal = useQuery({ queryKey: ["balance"], queryFn: () => api.get<Balance>("/billing/balance") });
  const tariffs = useQuery({ queryKey: ["my-tariff"], queryFn: () => api.get<{ items: ClientTariff[] }>("/billing/tariff") });
  const stats = useQuery({ queryKey: ["billing-stats"], queryFn: () => api.get<BillingStats>("/billing/stats") });
  const ledger = useQuery({
    queryKey: ["my-ledger", offset],
    queryFn: () => api.get<{ items: LedgerRow[] }>(withPage("/billing/ledger", offset)),
  });
  if (bal.isError) {
    return <ErrorBox error={bal.error} />;
  }
  if (!bal.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const items = ledger.data?.items ?? [];
  return (
    <div>
      <PageHeader title="Биллинг" />
      <div className="mb-4 grid gap-3 md:grid-cols-3">
        <Card>
          <div className="text-xs text-zinc-500">Доступно</div>
          <div className="mt-1 text-lg font-medium">{formatMoney(bal.data.available_balance, bal.data.currency)}</div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Заморожено</div>
          <div className="mt-1 text-lg font-medium">{formatMoney(bal.data.held_balance, bal.data.currency)}</div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Списано 24 ч / 7 д / 30 д</div>
          <div className="mt-1 text-sm">
            {formatMoney(stats.data?.["24h"]?.spent, bal.data.currency)} · {formatMoney(stats.data?.["7d"]?.spent, bal.data.currency)} ·{" "}
            {formatMoney(stats.data?.["30d"]?.spent, bal.data.currency)}
          </div>
        </Card>
      </div>
      <Card className="mb-4">
        <h2 className="mb-2 font-medium">Цены</h2>
        {(tariffs.data?.items ?? []).length === 0 ? <p className="text-sm text-zinc-500">не назначены</p> : null}
        <ul className="text-sm">
          {(tariffs.data?.items ?? []).map((t) => (
            <li key={t.product}>
              {productLabel[t.product] ?? t.product}: {formatMoney(t.sell_price, t.currency)} {priceUnit(t.product)}
            </li>
          ))}
        </ul>
      </Card>
      <Card className="mb-4">
        <h2 className="mb-2 font-medium">Исходящие SMS</h2>
        {["24h", "7d"].map((w) => (
          <p key={w} className="text-sm text-zinc-600">
            {w}: {Object.entries(stats.data?.[w]?.sms ?? {})
              .map(([k, v]) => `${k} ${v}`)
              .join(" · ") || "нет"}
          </p>
        ))}
      </Card>
      <Card className="mb-4">
        <h2 className="mb-2 font-medium">Проверки HLR и Silent SMS</h2>
        {["24h", "7d", "30d"].map((w) => (
          <p key={w} className="text-sm text-zinc-600">
            {w}: {Object.entries(stats.data?.[w]?.lookups ?? {})
              .map(([k, v]) => `${typeLabel[k as keyof typeof typeLabel] ?? k} ${v}`)
              .join(" · ") || "нет"}
          </p>
        ))}
      </Card>
      {ledger.isError ? <ErrorBox error={ledger.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Время</Th>
            <Th>Тип</Th>
            <Th>Сумма</Th>
            <Th>Комментарий</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((row) => (
            <tr key={row.id}>
              <Td>{row.created_at}</Td>
              <Td>{row.type}</Td>
              <Td>{formatMoney(row.amount, row.currency)}</Td>
              <Td>{row.description ?? "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!ledger.isLoading && items.length === 0 ? <EmptyState>Операций нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
    </div>
  );
}
