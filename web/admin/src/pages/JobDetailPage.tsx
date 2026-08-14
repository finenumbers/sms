import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Alert, Badge, Button, Card, EmptyState, ErrorBox, PAGE_SIZE, PageHeader, Pager, Table, Td, Th, statusTone, withPage, formatMoney } from "ui";
import { api, downloadAdminFile, type ClientRow, type LookupItem, type LookupJob } from "../api";
import { itemStatusLabel, jobStatusLabel, lookupInflight, resultLabel, sourceLabel, typeLabel, yn } from "../lookup";

export function JobDetailPage() {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const job = useQuery({
    queryKey: ["admin-lookup-job", id],
    queryFn: () => api.get<LookupJob>(`/lookups/jobs/${id}`),
    refetchInterval: (q) => (lookupInflight(q.state.data?.status) ? 2000 : false),
  });
  const items = useQuery({
    queryKey: ["admin-lookup-items", id, offset],
    queryFn: () => api.get<{ items: LookupItem[]; total: number }>(withPage(`/lookups/jobs/${id}/items`, offset)),
    refetchInterval: () => (lookupInflight(job.data?.status) ? 2000 : false),
    enabled: Boolean(id),
  });
  const clients = useQuery({
    queryKey: ["clients", "jobs-filter"],
    queryFn: () => api.get<{ items: ClientRow[] }>(withPage("/clients", 0, {}, 100)),
  });
  const exp = useMutation({
    mutationFn: () => downloadAdminFile(`/lookups/jobs/${id}/export`, `lookup-${job.data?.type ?? "check"}-${id}.xlsx`),
  });
  const finalize = useMutation({
    mutationFn: () => api.post<LookupJob>(`/lookups/jobs/${id}/finalize`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-lookup-job", id] });
      void qc.invalidateQueries({ queryKey: ["admin-lookup-items", id] });
      void qc.invalidateQueries({ queryKey: ["admin-lookup-jobs"] });
    },
  });

  if (job.isError) {
    return <ErrorBox error={job.error} />;
  }
  if (!job.data) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  const j = job.data;
  const hlr = j.type === "hlr";
  const rows = items.data?.items ?? [];
  const clientName = (clients.data?.items ?? []).find((c) => c.id === j.client_id)?.name;
  const inflight = lookupInflight(j.status);

  return (
    <div>
      <PageHeader
        title={`${typeLabel[j.type] ?? j.type} · ${jobStatusLabel[j.status] ?? j.status}`}
        actions={
          <div className="flex flex-wrap gap-2">
            <Link className="text-sm text-blue-700 hover:underline" to="/jobs">
              К списку
            </Link>
            <Button type="button" variant="secondary" disabled={exp.isPending || j.item_count === 0} onClick={() => exp.mutate()}>
              Скачать XLSX
            </Button>
            {inflight ? (
              <Button type="button" variant="secondary" disabled={finalize.isPending} onClick={() => finalize.mutate()}>
                Закрыть задание
              </Button>
            ) : null}
          </div>
        }
      />
      <p className="mb-3 text-sm text-zinc-500">
        Закрытие срабатывает, только если все номера уже в финале. Хвост принудительно не роняется.
      </p>
      <div className="mb-4 grid gap-3 md:grid-cols-4">
        <Card>
          <div className="text-xs text-zinc-500">Клиент</div>
          <div className="mt-1 text-sm">
            <Link className="text-blue-700 hover:underline" to={`/clients/${j.client_id}`}>
              {clientName ?? j.client_id}
            </Link>
          </div>
          <div className="mt-2 text-xs text-zinc-500">{sourceLabel[j.source] ?? j.source}</div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Статус</div>
          <div className="mt-1">
            <Badge tone={statusTone(j.status)}>{jobStatusLabel[j.status] ?? j.status}</Badge>
          </div>
          {j.tariff_plan_code ? <div className="mt-2 text-xs text-zinc-500">{j.tariff_plan_code}</div> : null}
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Номера</div>
          <div className="mt-1 text-sm">
            готово {j.success_count} · ошибки {j.failure_count} · всего {j.item_count}
          </div>
        </Card>
        <Card>
          <div className="text-xs text-zinc-500">Стоимость для клиента</div>
          <div className="mt-1 text-sm">
            оценка {formatMoney(j.estimated_cost, j.currency)}
            {j.actual_cost ? ` · списано ${formatMoney(j.actual_cost, j.currency)}` : null}
          </div>
        </Card>
      </div>
      {j.error_message ? <Alert className="mb-4">{j.error_message}</Alert> : null}
      {finalize.isSuccess && lookupInflight(finalize.data.status) ? (
        <Alert className="mb-4" tone="amber">
          Задание ещё не закрыто: есть незавершённые номера.
        </Alert>
      ) : null}
      {finalize.isSuccess && !lookupInflight(finalize.data.status) ? (
        <Alert className="mb-4" tone="green">
          Задание закрыто
        </Alert>
      ) : null}
      {finalize.isError ? (
        <div className="mb-4">
          <ErrorBox error={finalize.error} />
        </div>
      ) : null}
      {exp.isError ? (
        <div className="mb-4">
          <ErrorBox error={exp.error} />
        </div>
      ) : null}
      {items.isError ? (
        <div className="mb-4">
          <ErrorBox error={items.error} />
        </div>
      ) : null}
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
