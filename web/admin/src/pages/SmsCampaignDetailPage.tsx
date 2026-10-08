import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge, Button, Card, EmptyState, ErrorBox, PageHeader, Table, Td, Th, formatDateTime, formatMoney, statusTone } from "ui";
import {
  api,
  type SmsCampaignBilling,
  type SmsCampaignDetail,
  type SmsCampaignSummary,
  type SmsPage,
  type SmsRecipient,
} from "../api";
import { billingStateLabel, campaignInflight, campaignStatusLabel, recipientStatusLabel, smsStatusLabel } from "../sms";

export function SmsCampaignDetailPage() {
  const { id = "" } = useParams();
  const [cursor, setCursor] = useState("");
  const [back, setBack] = useState<string[]>([]);
  useEffect(() => {
    setCursor("");
    setBack([]);
  }, [id]);
  const campaign = useQuery({
    queryKey: ["admin-sms-campaign", id],
    queryFn: () => api.get<SmsCampaignDetail>(`/sms/campaigns/${id}`),
    refetchInterval: (q) => (campaignInflight(q.state.data?.status) ? 4000 : false),
  });
  const summary = useQuery({
    queryKey: ["admin-sms-campaign-summary", id],
    queryFn: () => api.get<SmsCampaignSummary>(`/sms/campaigns/${id}/summary`),
    refetchInterval: () => (campaignInflight(campaign.data?.status) ? 15000 : false),
  });
  const billing = useQuery({
    queryKey: ["admin-sms-campaign-billing", id],
    queryFn: () => api.get<SmsCampaignBilling>(`/sms/campaigns/${id}/billing`),
    staleTime: Infinity,
  });
  const recipients = useQuery({
    queryKey: ["admin-sms-recipients", id, cursor],
    queryFn: () => api.get<SmsPage<SmsRecipient>>(`/sms/campaigns/${id}/recipients?limit=50${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`),
    refetchInterval: () => (campaignInflight(campaign.data?.status) ? 4000 : false),
  });
  const row = campaign.data;
  const recItems = recipients.data?.items ?? [];

  return (
    <div>
      <PageHeader
        title={row ? `Рассылка · ${campaignStatusLabel[row.status] ?? row.status}` : "Рассылка"}
        actions={
          <Link className="text-sm text-blue-700 hover:underline" to="/sms-jobs">
            К списку
          </Link>
        }
      />
      {campaign.isError ? <ErrorBox error={campaign.error} /> : null}
      {row ? (
        <>
          <p className="mb-3 text-sm text-zinc-500">{row.counters_note}</p>
          <div className="mb-4 grid gap-3 md:grid-cols-4">
            <Card>
              <div className="text-xs text-zinc-500">Клиент</div>
              <div className="mt-1 text-sm">
                <Link className="text-blue-700 hover:underline" to={`/clients/${row.client_id}`}>
                  {row.client_name || row.client_id}
                </Link>
              </div>
              <div className="mt-2 text-xs text-zinc-500">от {row.from}</div>
            </Card>
            <Card>
              <div className="text-xs text-zinc-500">Статус</div>
              <div className="mt-1">
                <Badge tone={statusTone(row.status)}>{campaignStatusLabel[row.status] ?? row.status}</Badge>
              </div>
              <div className="mt-2 text-xs text-zinc-500">{row.created_by_email || "—"}</div>
            </Card>
            <Card>
              <div className="text-xs text-zinc-500">Счётчики</div>
              <div className="mt-1 text-sm">
                {row.total_count} всего · {row.accepted_count} принято · {row.delivered_count} доставлено · {row.failed_count} ошибок
              </div>
            </Card>
            <Card>
              <div className="text-xs text-zinc-500">Суммы по тарифу</div>
              <div className="mt-1 text-sm">
                {billing.data
                  ? `списано ${formatMoney(billing.data.captured, billing.data.currency)} · холд ${formatMoney(billing.data.held, billing.data.currency)} · снято ${formatMoney(billing.data.released, billing.data.currency)}`
                  : "считаем…"}
              </div>
              <Button className="mt-2" type="button" variant="secondary" onClick={() => void billing.refetch()}>
                Обновить суммы
              </Button>
            </Card>
          </div>
          <Card className="mb-4">
            <div className="text-xs text-zinc-500">Текст</div>
            <pre className="mt-2 whitespace-pre-wrap font-sans text-sm">{row.text}</pre>
            <div className="mt-2 text-xs text-zinc-500">
              создана {formatDateTime(row.created_at)} · обновлена {formatDateTime(row.updated_at)}
            </div>
          </Card>
        </>
      ) : null}
      {summary.data ? (
        <p className="mb-3 text-sm text-zinc-600">
          Живая очередь: ожидает {summary.data.pending} · поставлено {summary.data.enqueued} · пропущено {summary.data.skipped} · ошибка получателя {summary.data.failed}
        </p>
      ) : null}
      {summary.isError ? <ErrorBox error={summary.error} /> : null}
      {recipients.isError ? <ErrorBox error={recipients.error} /> : null}
      <div className="mb-3 flex gap-2">
        <Button
          type="button"
          variant="secondary"
          disabled={back.length === 0}
          onClick={() => {
            const prev = back[back.length - 1] ?? "";
            setBack((stack) => stack.slice(0, -1));
            setCursor(prev);
          }}
        >
          Назад
        </Button>
        <Button
          type="button"
          variant="secondary"
          disabled={!recipients.data?.has_more || !recipients.data.next_cursor}
          onClick={() => {
            const next = recipients.data?.next_cursor;
            if (!next) {
              return;
            }
            setBack((stack) => [...stack, cursor]);
            setCursor(next);
          }}
        >
          Дальше
        </Button>
      </div>
      <Table>
        <thead>
          <tr>
            <Th fit>Номер</Th>
            <Th fit>Получатель</Th>
            <Th fit>SMS</Th>
            <Th fit>Провайдер</Th>
            <Th fit>Сегменты</Th>
            <Th fit>Сумма</Th>
            <Th fit>Биллинг</Th>
            <Th>Ошибка очереди</Th>
          </tr>
        </thead>
        <tbody>
          {recItems.map((item) => (
            <tr key={item.id}>
              <Td fit>
                {item.message_id ? (
                  <Link className="text-blue-700 hover:underline" to={`/sms-jobs/messages/${item.message_id}`}>
                    {item.to}
                  </Link>
                ) : (
                  item.to
                )}
              </Td>
              <Td fit>{recipientStatusLabel[item.status] ?? item.status}</Td>
              <Td fit>{item.message_status ? (smsStatusLabel[item.message_status] ?? item.message_status) : "—"}</Td>
              <Td fit>{item.provider_status || item.provider_sms_id || "—"}</Td>
              <Td fit>{item.billed_segments ?? "—"}</Td>
              <Td fit>{item.billed_amount ? formatMoney(item.billed_amount, item.currency) : "—"}</Td>
              <Td fit>{billingStateLabel[item.billing_state] ?? item.billing_state}</Td>
              <Td>{item.last_error || "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!recipients.isLoading && recItems.length === 0 ? <EmptyState>Получателей нет</EmptyState> : null}
    </div>
  );
}
