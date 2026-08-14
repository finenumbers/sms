import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Alert, Badge, Button, Card, EmptyState, PAGE_SIZE, PageHeader, Pager, Table, Td, Th, statusTone, withPage, formatMoney } from "ui";
import { api, downloadClientFile, type LookupItem, type LookupJob } from "../api";
import { itemStatusLabel, jobStatusLabel, lookupError, lookupInflight, resultLabel, sourceLabel, typeLabel } from "../lookup";

function yn(v?: boolean) {
  if (v == null) {
    return "—";
  }
  return v ? "да" : "нет";
}

export function LookupDetailPage() {
  const { id = "" } = useParams();
  const [offset, setOffset] = useState(0);
  const job = useQuery({
    queryKey: ["lookup-job", id],
    queryFn: () => api.get<LookupJob>(`/lookups/jobs/${id}`),
    refetchInterval: (q) => (lookupInflight(q.state.data?.status) ? 2000 : false),
  });
  const items = useQuery({
    queryKey: ["lookup-items", id, offset],
    queryFn: () => api.get<{ items: LookupItem[]; total: number }>(withPage(`/lookups/jobs/${id}/items`, offset)),
    refetchInterval: () => (lookupInflight(job.data?.status) ? 2000 : false),
    enabled: Boolean(id),
  });
  const exp = useMutation({
    mutationFn: () => downloadClientFile(`/lookups/jobs/${id}/export`, `lookup-${job.data?.type ?? "check"}-${id}.xlsx`),
  });

  if (job.isError) {
    return <Alert>{lookupError(job.error)}</Alert>;
  }
  if (!job.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const j = job.data;
  const hlr = j.type === "hlr";
  const rows = items.data?.items ?? [];

  return (
    <div>
      <PageHeader
        title={`${typeLabel[j.type] ?? j.type} · ${jobStatusLabel[j.status] ?? j.status}`}
        actions={
          <div className="flex gap-2">
            <Link className="text-sm text-blue-700 hover:underline" to="/lookups">
              К списку
            </Link>
            <Button type="button" variant="secondary" disabled={exp.isPending || j.item_count === 0} onClick={() => exp.mutate()}>
              Скачать XLSX
            </Button>
          </div>
        }
      />
      <div className="mb-4 grid gap-3 md:grid-cols-3">
        <Card>
          <div className="text-xs text-zinc-500">Статус</div>
          <div className="mt-1">
            <Badge tone={statusTone(j.status)}>{jobStatusLabel[j.status] ?? j.status}</Badge>
          </div>
          <div className="mt-2 text-xs text-zinc-500">{sourceLabel[j.source] ?? j.source}</div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Номера</div>
          <div className="mt-1 text-sm">
            готово {j.success_count} · ошибки {j.failure_count} · всего {j.item_count}
          </div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Стоимость</div>
          <div className="mt-1 text-sm">
            оценка {formatMoney(j.estimated_cost, j.currency)}
            {j.actual_cost ? ` · списано ${formatMoney(j.actual_cost, j.currency)}` : null}
          </div>
        </Card>
      </div>
      {j.error_message ? <Alert className="mb-4">{j.error_message}</Alert> : null}
      {exp.isError ? <Alert className="mb-4">{lookupError(exp.error)}</Alert> : null}
      {items.isError ? <Alert className="mb-4">{lookupError(items.error)}</Alert> : null}
      <Table>
        <thead>
          <tr>
            <Th>Номер</Th>
            <Th>Статус</Th>
            <Th>Результат</Th>
            {hlr ? <Th>Оператор</Th> : null}
            {hlr ? <Th>IMSI</Th> : null}
            {hlr ? <Th>MSC</Th> : null}
            {hlr ? <Th>Роуминг</Th> : null}
            <Th>Ошибка</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map((it) => (
            <tr key={it.id}>
              <Td>
                <code>{it.phone}</code>
              </Td>
              <Td>
                <Badge tone={statusTone(it.status)}>{itemStatusLabel[it.status] ?? it.status}</Badge>
              </Td>
              <Td>{it.result_status ? resultLabel[it.result_status] ?? it.result_status : yn(it.is_reachable)}</Td>
              {hlr ? <Td>{it.operator_name ?? "—"}</Td> : null}
              {hlr ? <Td>{it.imsi ?? "—"}</Td> : null}
              {hlr ? <Td>{it.msc ?? "—"}</Td> : null}
              {hlr ? <Td>{yn(it.roaming)}</Td> : null}
              <Td>{it.error_message ?? "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!items.isLoading && rows.length === 0 ? <EmptyState>Номеров пока нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={rows.length} onChange={setOffset} />
    </div>
  );
}
