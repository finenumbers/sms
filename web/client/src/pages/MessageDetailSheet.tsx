import { Badge, ErrorBox, Sheet, statusTone, formatDateTime, formatMoney } from "ui";
import type { Message } from "../api";

export const smsInflight = new Set(["queued", "accepted", "sent"]);

function MessageDetailBody({ m }: { m: Message }) {
  return (
    <>
      <div className="mb-3">
        <Badge tone={statusTone(m.status)}>{m.status}</Badge>
        {smsInflight.has(m.status) ? <span className="ml-2 text-xs text-zinc-500">опрос каждые 2 с</span> : null}
      </div>
      <dl className="grid grid-cols-[7rem_1fr] gap-x-3 gap-y-2 text-sm">
        <dt className="text-zinc-500">Откуда</dt>
        <dd>
          <code>{m.from}</code>
        </dd>
        <dt className="text-zinc-500">Куда</dt>
        <dd>
          <code>{m.to}</code>
        </dd>
        <dt className="text-zinc-500">Текст</dt>
        <dd className="whitespace-pre-wrap">{m.text}</dd>
        <dt className="text-zinc-500">Создано</dt>
        <dd>{formatDateTime(m.created_at)}</dd>
        <dt className="text-zinc-500">ID у провайдера</dt>
        <dd>{m.provider_sms_id ?? "—"}</dd>
        {m.billed_segments != null ? (
          <>
            <dt className="text-zinc-500">Сегменты / сумма</dt>
            <dd>
              {m.billed_segments} PDU
              {m.billed_amount ? ` · ${formatMoney(m.billed_amount, m.currency)}` : ""}
              {m.billing_action ? ` · ${m.billing_action}` : m.billed_segments ? " · заморожено" : ""}
            </dd>
          </>
        ) : null}
        {m.pdu_count != null ? (
          <>
            <dt className="text-zinc-500">PDU провайдера</dt>
            <dd>{m.pdu_count}</dd>
          </>
        ) : null}
        {m.status === "failed" ? (
          <>
            <dt className="text-zinc-500">Ошибка провайдера</dt>
            <dd className="whitespace-pre-wrap font-mono text-xs">{m.provider_status ?? "—"}</dd>
          </>
        ) : null}
      </dl>
    </>
  );
}

export function MessageDetailSheet({
  open,
  onOpenChange,
  message,
  loading,
  error,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  message: Message | null;
  loading?: boolean;
  error?: unknown;
}) {
  const title = message ? `${message.from} → ${message.to}` : "SMS";
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={title} description="Детали сообщения">
      {loading && !message ? <p className="text-sm text-zinc-500">Загрузка…</p> : null}
      {error ? <ErrorBox error={error} /> : null}
      {message ? <MessageDetailBody m={message} /> : null}
    </Sheet>
  );
}
