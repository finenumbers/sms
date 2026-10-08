import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { Badge, Card, ErrorBox, PageHeader, formatDateTime, formatMoney, statusTone } from "ui";
import { api, type SmsMessageDetail } from "../api";
import { billingStateLabel, sendJobStatusLabel, smsInflight, smsStatusLabel } from "../sms";

export function SmsMessageDetailPage() {
  const { id = "" } = useParams();
  const message = useQuery({
    queryKey: ["admin-sms-message", id],
    queryFn: () => api.get<SmsMessageDetail>(`/sms/messages/${id}`),
    refetchInterval: (q) => (smsInflight(q.state.data?.status) ? 4000 : false),
  });
  const row = message.data;
  const back = row?.direction === "inbound" ? "/sms-jobs?kind=inbound" : "/sms-jobs?kind=outbound";

  return (
    <div>
      <PageHeader
        title={row ? `${row.direction === "inbound" ? "Входящее" : "Исходящее"} · ${smsStatusLabel[row.status] ?? row.status}` : "SMS"}
        actions={
          <Link className="text-sm text-blue-700 hover:underline" to={back}>
            К списку
          </Link>
        }
      />
      {message.isError ? <ErrorBox error={message.error} /> : null}
      {row ? (
        <div className="grid gap-3">
          <Card>
            <div className="mb-2">
              <Badge tone={statusTone(row.status)}>{smsStatusLabel[row.status] ?? row.status}</Badge>
            </div>
            <dl className="grid gap-2 text-sm md:grid-cols-2">
              <Field label="Клиент">
                {row.client_id ? (
                  <Link className="text-blue-700 hover:underline" to={`/clients/${row.client_id}`}>
                    {row.client_name || row.client_id}
                  </Link>
                ) : (
                  "без клиента"
                )}
              </Field>
              <Field label="Направление">{row.direction === "inbound" ? "входящее" : "исходящее"}</Field>
              <Field label="От">{row.from}</Field>
              <Field label="Кому">{row.to}</Field>
              <Field label="Провайдер">{row.provider}</Field>
              <Field label="ID провайдера">{row.provider_sms_id || "—"}</Field>
              <Field label="Статус провайдера">{row.provider_status || "—"}</Field>
              <Field label="PDU">{row.pdu_count ?? "—"}</Field>
              <Field label="Рассылка">
                {row.campaign_id ? (
                  <Link className="text-blue-700 hover:underline" to={`/sms-jobs/campaigns/${row.campaign_id}`}>
                    открыть
                  </Link>
                ) : (
                  "—"
                )}
              </Field>
              <Field label="Ключ идемпотентности">{row.idempotency_key || "—"}</Field>
              <Field label="Создано">{formatDateTime(row.created_at)}</Field>
              <Field label="Принято">{row.accepted_at ? formatDateTime(row.accepted_at) : "—"}</Field>
              <Field label="Отправлено">{row.sent_at ? formatDateTime(row.sent_at) : "—"}</Field>
              <Field label="Доставлено">{row.delivered_at ? formatDateTime(row.delivered_at) : "—"}</Field>
              <Field label="Ошибка">{row.failed_at ? formatDateTime(row.failed_at) : "—"}</Field>
            </dl>
            <div className="mt-3 text-xs text-zinc-500">Текст</div>
            <pre className="mt-1 whitespace-pre-wrap font-sans text-sm">{row.text}</pre>
          </Card>
          <Card>
            <div className="text-xs text-zinc-500">Биллинг</div>
            <div className="mt-1 text-sm">
              {billingStateLabel[row.billing_state] ?? row.billing_state}
              {row.unit_sell_price ? ` · ${formatMoney(row.unit_sell_price, row.currency)} за сегмент` : ""}
              {row.billed_segments ? ` · сегментов ${row.billed_segments}` : ""}
              {row.billed_amount ? ` · ${formatMoney(row.billed_amount, row.currency)}` : ""}
              {row.tariff_plan_code ? ` · тариф ${row.tariff_plan_code}` : ""}
            </div>
            {row.tariff_plan_id ? <div className="mt-1 text-xs text-zinc-500">{row.tariff_plan_id}</div> : null}
          </Card>
          <Card>
            <div className="text-xs text-zinc-500">Очередь отправки</div>
            {row.send_job ? (
              <dl className="mt-2 grid gap-2 text-sm md:grid-cols-2">
                <Field label="Статус">{sendJobStatusLabel[row.send_job.status] ?? row.send_job.status}</Field>
                <Field label="Попытка">{row.send_job.attempt}</Field>
                <Field label="Доступна">{formatDateTime(row.send_job.available_at)}</Field>
                <Field label="Заблокирована">{row.send_job.locked_at ? formatDateTime(row.send_job.locked_at) : "—"}</Field>
                <Field label="Кем">{row.send_job.locked_by || "—"}</Field>
                <Field label="Ошибка">{row.send_job.last_error || "—"}</Field>
              </dl>
            ) : (
              <p className="mt-1 text-sm text-zinc-600">Очереди отправки нет.</p>
            )}
          </Card>
          <Card>
            <div className="text-xs text-zinc-500">Попытки провайдера</div>
            {row.attempts.length === 0 ? <p className="mt-1 text-sm text-zinc-600">Попыток нет.</p> : null}
            <ul className="mt-2 space-y-3">
              {row.attempts.map((attempt) => (
                <li key={attempt.id} className="border-t border-zinc-200 pt-2 text-sm">
                  <div>
                    #{attempt.attempt} · {attempt.error_kind || "—"} · HTTP {attempt.http_status ?? "—"} · {attempt.latency_ms ?? "—"} мс · {formatDateTime(attempt.created_at)}
                  </div>
                  {attempt.response_body ? <pre className="mt-1 whitespace-pre-wrap text-xs text-zinc-700">{attempt.response_body}</pre> : null}
                </li>
              ))}
            </ul>
          </Card>
          <Card>
            <div className="text-xs text-zinc-500">Колбэки</div>
            {row.callbacks.length === 0 ? <p className="mt-1 text-sm text-zinc-600">Колбэков нет.</p> : null}
            <ul className="mt-2 space-y-3">
              {row.callbacks.map((cb) => (
                <li key={cb.id} className="border-t border-zinc-200 pt-2 text-sm">
                  <div>
                    {cb.kind} · {formatDateTime(cb.created_at)}
                    {cb.processed_at ? ` · разобран ${formatDateTime(cb.processed_at)}` : ""}
                  </div>
                  {cb.parsed ? <pre className="mt-1 whitespace-pre-wrap text-xs text-zinc-700">{JSON.stringify(cb.parsed, null, 2)}</pre> : null}
                </li>
              ))}
            </ul>
          </Card>
        </div>
      ) : null}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs text-zinc-500">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
