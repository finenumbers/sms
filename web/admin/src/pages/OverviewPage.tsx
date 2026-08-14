import { Link } from "react-router-dom";
import { Card, ErrorBox, PageHeader } from "ui";
import { formatMoney } from "ui";
import { useQuery } from "@tanstack/react-query";
import { api, type BillingOverview } from "../api";

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <div className="text-xs text-zinc-500">{label}</div>
      <div className="mt-1 text-lg font-medium">{value}</div>
    </Card>
  );
}

function counts(map?: Record<string, number>) {
  if (!map) return "—";
  return Object.entries(map)
    .map(([k, v]) => `${k}: ${v}`)
    .join(" · ");
}

export function OverviewPage() {
  const q = useQuery({ queryKey: ["billing-overview"], queryFn: () => api.get<BillingOverview>("/billing/overview") });
  if (q.isError) {
    return <ErrorBox error={q.error} />;
  }
  if (!q.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const d = q.data;
  return (
    <div>
      <PageHeader title="Обзор" />
      <div className="mb-4 grid gap-3 md:grid-cols-3">
        <Stat label="Доступно у клиентов" value={formatMoney(d.available_total)} />
        <Stat label="Заморожено" value={formatMoney(d.held_total)} />
        <Stat label="Списано за 24 ч" value={formatMoney(d.spent_24h)} />
        <Stat label="Списано за 7 д" value={formatMoney(d.spent_7d)} />
        <Stat label="Открытые HOLD" value={String(d.open_holds)} />
        <Stat label="PDU за 24 ч (снимок)" value={String(d.billed_segments_24h)} />
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <Card>
          <h2 className="mb-2 font-medium">SMS за 24 ч</h2>
          <p className="text-sm text-zinc-600">{counts(d.sms_24h)}</p>
          <p className="mt-2 text-xs text-zinc-500">
            7… / мир: {d.sms_by_product_24h.sms_domestic ?? 0} / {d.sms_by_product_24h.sms_international ?? 0}
          </p>
        </Card>
        <Card>
          <h2 className="mb-2 font-medium">SMS за 7 д</h2>
          <p className="text-sm text-zinc-600">{counts(d.sms_7d)}</p>
          <p className="mt-2 text-sm">
            <Link className="text-blue-700 hover:underline" to="/clients">
              клиентов ниже порога ({formatMoney(d.low_balance_threshold)}): {d.low_balance_clients}
            </Link>
          </p>
        </Card>
      </div>
    </div>
  );
}
