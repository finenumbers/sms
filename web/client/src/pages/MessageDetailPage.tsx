import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { Badge, Card, ErrorBox, PageHeader, pollStatus, statusTone, formatDateTime, formatMoney } from "ui";
import { api, type Message } from "../api";

export function MessageDetailPage() {
  const { id = "" } = useParams();
  const q = useQuery({
    queryKey: ["message", id],
    queryFn: () => api.get<Message>(`/messages/${id}`),
    refetchInterval: pollStatus<Message>(),
  });
  if (q.isError) {
    return <ErrorBox error={q.error} />;
  }
  if (!q.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const m = q.data;
  return (
    <div>
      <PageHeader
        title="Статус SMS"
        actions={
          <Link className="text-sm text-blue-700" to="/messages">
            ← к списку
          </Link>
        }
      />
      <Card>
        <div className="mb-3">
          <Badge tone={statusTone(m.status)}>{m.status}</Badge>
          {["queued", "accepted", "sent"].includes(m.status) ? (
            <span className="ml-2 text-xs text-zinc-500">опрос каждые 2 с</span>
          ) : null}
        </div>
        <dl className="grid gap-2 text-sm md:grid-cols-2">
          <div>
            <dt className="text-zinc-500">Откуда</dt>
            <dd>
              <code>{m.from}</code>
            </dd>
          </div>
          <div>
            <dt className="text-zinc-500">Куда</dt>
            <dd>
              <code>{m.to}</code>
            </dd>
          </div>
          <div className="md:col-span-2">
            <dt className="text-zinc-500">Текст</dt>
            <dd className="whitespace-pre-wrap">{m.text}</dd>
          </div>
          <div>
            <dt className="text-zinc-500">Создано</dt>
            <dd>{formatDateTime(m.created_at)}</dd>
          </div>
          <div>
            <dt className="text-zinc-500">ID у провайдера</dt>
            <dd>{m.provider_sms_id ?? "—"}</dd>
          </div>
          {m.billed_segments != null ? (
            <div>
              <dt className="text-zinc-500">Сегменты / сумма</dt>
              <dd>
                {m.billed_segments} PDU
                {m.billed_amount ? ` · ${formatMoney(m.billed_amount, m.currency)}` : ""}
                {m.billing_action ? ` · ${m.billing_action}` : m.billed_segments ? " · заморожено" : ""}
              </dd>
            </div>
          ) : null}
          {m.pdu_count != null ? (
            <div>
              <dt className="text-zinc-500">PDU провайдера</dt>
              <dd>{m.pdu_count}</dd>
            </div>
          ) : null}
          {m.status === "failed" ? (
            <div className="md:col-span-2">
              <dt className="text-zinc-500">Ошибка провайдера</dt>
              <dd className="whitespace-pre-wrap font-mono text-xs">{m.provider_status ?? "—"}</dd>
            </div>
          ) : null}
        </dl>
      </Card>
    </div>
  );
}
