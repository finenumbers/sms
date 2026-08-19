import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, Input, InvalidList, PAGE_SIZE, PageHeader, Pager, Table, Td, Textarea, Th, cn, pollStatus, statusTone, withPage, formatMoney } from "ui";
import { api, type Campaign, type Message, type Recipient } from "../api";
import { MessageDetailSheet } from "./MessageDetailSheet";

type RecipientReport = {
  added: number;
  duplicates: number;
  total: number;
  encoding?: string;
  invalid?: { line?: number; value?: string; error: string }[];
};

function recipientMessageSeed(c: Campaign, r: Recipient): Message | null {
  if (!r.message_id) {
    return null;
  }
  return {
    id: r.message_id,
    direction: "outbound",
    from: c.from,
    to: r.to,
    text: c.text,
    status: r.message_status ?? "queued",
    created_at: r.created_at,
  };
}

export function CampaignDetailPage() {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const [selectedMessageId, setSelectedMessageId] = useState<string | null>(null);
  const [snapshot, setSnapshot] = useState<Message | null>(null);
  const q = useQuery({
    queryKey: ["campaign", id],
    queryFn: () => api.get<Campaign>(`/campaigns/${id}`),
    refetchInterval: pollStatus<Campaign>(),
  });
  const rec = useQuery({
    queryKey: ["recipients", id, offset],
    queryFn: () => api.get<{ items: Recipient[] }>(withPage(`/campaigns/${id}/recipients`, offset)),
    refetchInterval: 4000,
  });
  const estimate = useQuery({
    queryKey: ["campaign-estimate", id, q.data?.text, q.data?.total_count],
    queryFn: () =>
      api.get<{ billed: boolean; domestic: number; international: number; segments: number; total: string; currency: string }>(
        `/campaigns/${id}/estimate`,
      ),
    enabled: Boolean(q.data && q.data.status === "draft"),
  });
  const recItems = rec.data?.items ?? [];
  const selected = recItems.find((r) => r.message_id === selectedMessageId);
  const seed = (q.data && selected ? recipientMessageSeed(q.data, selected) : null) ?? snapshot;
  const detail = useQuery({
    queryKey: ["message", selectedMessageId],
    queryFn: () => api.get<Message>(`/messages/${selectedMessageId}`),
    enabled: Boolean(selectedMessageId),
    refetchInterval: pollStatus<Message>(),
  });
  const message = detail.data ?? seed ?? null;
  const [list, setList] = useState("");
  const [from, setFrom] = useState("");
  const [text, setText] = useState("");
  const [report, setReport] = useState<RecipientReport | null>(null);

  useEffect(() => {
    if (q.data) {
      setFrom(q.data.from);
      setText(q.data.text);
    }
  }, [q.data]);

  const add = useMutation({
    mutationFn: () =>
      api.post<RecipientReport>(`/campaigns/${id}/recipients`, {
        recipients: list.split(/[\s,;]+/).filter(Boolean),
      }),
    onSuccess: (r) => {
      setList("");
      setReport(r);
      setOffset(0);
      void qc.invalidateQueries({ queryKey: ["campaign", id] });
      void qc.invalidateQueries({ queryKey: ["recipients", id] });
    },
  });
  const upload = useMutation({
    mutationFn: (file: File) => api.upload<RecipientReport>(`/campaigns/${id}/recipients/upload`, file),
    onSuccess: (r) => {
      setReport(r);
      setOffset(0);
      void qc.invalidateQueries({ queryKey: ["campaign", id] });
      void qc.invalidateQueries({ queryKey: ["recipients", id] });
    },
  });
  const patch = useMutation({
    mutationFn: () => api.patch<Campaign>(`/campaigns/${id}`, { from, text }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["campaign", id] }),
  });
  const start = useMutation({
    mutationFn: () => api.post(`/campaigns/${id}/start`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["campaign", id] }),
  });
  const cancel = useMutation({
    mutationFn: () => api.post(`/campaigns/${id}/cancel`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["campaign", id] }),
  });
  const del = useMutation({
    mutationFn: () => api.delete(`/campaigns/${id}`),
    onSuccess: () => {
      window.location.href = "/campaigns";
    },
  });

  if (q.isError) {
    return <ErrorBox error={q.error} />;
  }
  if (!q.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const c = q.data;
  const draft = c.status === "draft";
  const running = c.status === "queued" || c.status === "running";
  const items = recItems;
  return (
    <div>
      <PageHeader
        title="Рассылка"
        actions={
          <Link className="text-sm text-blue-700" to="/campaigns">
            ← к списку
          </Link>
        }
      />
      <Card className="mb-4">
        <div className="mb-2 flex items-center gap-2">
          <Badge tone={statusTone(c.status)}>{c.status}</Badge>
          {running ? <span className="text-xs text-zinc-500">опрос статуса</span> : null}
        </div>
        {draft ? (
          <>
            <Field label="От">
              <Input value={from} onChange={(e) => setFrom(e.target.value)} />
            </Field>
            <Field label="Текст">
              <Textarea value={text} onChange={(e) => setText(e.target.value)} rows={3} maxLength={1000} />
            </Field>
            <Button type="button" className="mb-3" onClick={() => patch.mutate()} disabled={patch.isPending}>
              Сохранить черновик
            </Button>
            {patch.isError ? <ErrorBox error={patch.error} /> : null}
          </>
        ) : (
          <>
            <p className="text-sm">
              Отправитель <code>{c.from}</code>
            </p>
            <p className="mt-2 whitespace-pre-wrap text-sm">{c.text}</p>
          </>
        )}
        <p className="mt-2 text-sm text-zinc-600">
          всего {c.total_count} · принято {c.accepted_count} · доставлено {c.delivered_count} · ошибки {c.failed_count + (c.recipients?.failed ?? 0)}
        </p>
        {estimate.data ? (
          <p className="mt-2 text-xs text-zinc-500">
            оценка: {estimate.data.segments} PDU × {estimate.data.domestic + estimate.data.international} получателей
            {` ≈ ${formatMoney(estimate.data.total, estimate.data.currency)}`}
          </p>
        ) : null}
        {estimate.isError ? <div className="mt-2"><ErrorBox error={estimate.error} /></div> : null}
        <div className="mt-3 flex flex-wrap gap-2">
          {draft ? (
            <Button type="button" onClick={() => start.mutate()} disabled={start.isPending}>
              Запустить
            </Button>
          ) : null}
          {running ? (
            <Button variant="secondary" type="button" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
              Отменить
            </Button>
          ) : null}
          {draft ? (
            <Button
              variant="danger"
              type="button"
              onClick={() => {
                if (window.confirm("Удалить черновик рассылки?")) {
                  del.mutate();
                }
              }}
            >
              Удалить черновик
            </Button>
          ) : null}
        </div>
        {start.isError ? <div className="mt-2"><ErrorBox error={start.error} /></div> : null}
        {cancel.isError ? <div className="mt-2"><ErrorBox error={cancel.error} /></div> : null}
      </Card>
      {draft ? (
        <Card className="mb-4">
          <h2 className="mb-3 font-medium">Получатели</h2>
          <Field label="Список MSISDN">
            <Textarea value={list} onChange={(e) => setList(e.target.value)} rows={4} placeholder="7… по одному в строке" />
          </Field>
          <div className="mb-3">
            <input
              type="file"
              accept=".csv,text/csv,text/plain"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) upload.mutate(f);
              }}
            />
          </div>
          {report ? (
            <p className="mb-2 text-sm text-zinc-600">
              добавлено {report.added}, дубли {report.duplicates}, всего {report.total}
              {report.encoding ? `, ${report.encoding}` : ""}
            </p>
          ) : null}
          <InvalidList rows={report?.invalid} />
          {add.isError ? <ErrorBox error={add.error} /> : null}
          {upload.isError ? <ErrorBox error={upload.error} /> : null}
          <Button type="button" onClick={() => add.mutate()} disabled={!list.trim() || add.isPending}>
            Добавить
          </Button>
        </Card>
      ) : null}
      <Table>
        <thead>
          <tr>
            <Th fit>Получатель</Th>
            <Th fit>Статус</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((r) => {
            const openable = Boolean(r.message_id);
            return (
              <tr
                key={r.id}
                className={cn(
                  openable && "cursor-pointer hover:bg-zinc-50",
                  openable && selectedMessageId === r.message_id && "bg-zinc-100 hover:bg-zinc-100",
                )}
                onClick={() => {
                  if (!r.message_id) {
                    return;
                  }
                  const next = recipientMessageSeed(c, r);
                  setSelectedMessageId(r.message_id);
                  setSnapshot(next);
                }}
              >
                <Td fit>
                  <code>{r.to}</code>
                </Td>
                <Td fit>
                  <Badge tone={statusTone(r.message_status ?? r.status)}>{r.message_status ?? r.status}</Badge>
                </Td>
              </tr>
            );
          })}
        </tbody>
      </Table>
      {!rec.isLoading && items.length === 0 ? <EmptyState>Получателей нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
      <MessageDetailSheet
        open={selectedMessageId != null}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedMessageId(null);
            setSnapshot(null);
          }
        }}
        message={message}
        loading={detail.isFetching && !message}
        error={detail.isError ? detail.error : undefined}
      />
    </div>
  );
}
