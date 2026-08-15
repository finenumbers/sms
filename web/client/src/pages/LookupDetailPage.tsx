import { useInfiniteQuery, useMutation, useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import {
  Alert,
  Badge,
  Button,
  Card,
  EmptyState,
  INFINITE_PAGE_SIZE,
  InfiniteSentinel,
  PageHeader,
  Table,
  Td,
  Th,
  statusTone,
  withPage,
  formatMoney,
} from "ui";
import { api, downloadClientFile, type LookupItem, type LookupJob } from "../api";
import { itemStatusLabel, jobStatusLabel, lookupError, lookupInflight, resultLabel, sourceLabel, typeLabel } from "../lookup";

function yn(v?: boolean) {
  if (v == null) {
    return "—";
  }
  return v ? "да" : "нет";
}

function dash(v?: string | null) {
  return v ? v : "—";
}

export function LookupDetailPage() {
  const { id = "" } = useParams();
  const job = useQuery({
    queryKey: ["lookup-job", id],
    queryFn: () => api.get<LookupJob>(`/lookups/jobs/${id}`),
    refetchInterval: (q) => (lookupInflight(q.state.data?.status) ? 2000 : false),
  });
  const items = useInfiniteQuery({
    queryKey: ["lookup-items", id],
    queryFn: ({ pageParam }) =>
      api.get<{ items: LookupItem[]; total: number }>(withPage(`/lookups/jobs/${id}/items`, pageParam, {}, INFINITE_PAGE_SIZE)),
    initialPageParam: 0,
    getNextPageParam: (last, _pages, lastParam) => {
      const next = lastParam + last.items.length;
      return next < last.total ? next : undefined;
    },
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
  const rows = items.data?.pages.flatMap((p) => p.items) ?? [];

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
            <Th fit>Номер</Th>
            <Th fit>Статус</Th>
            <Th fit>Результат</Th>
            {hlr ? <Th fit>Оператор</Th> : null}
            {hlr ? <Th fit>Страна</Th> : null}
            {hlr ? <Th fit>Регион</Th> : null}
            {hlr ? <Th fit>MCC</Th> : null}
            {hlr ? <Th fit>MNC</Th> : null}
            {hlr ? <Th fit>IMSI</Th> : null}
            {hlr ? <Th fit>MSC</Th> : null}
            {hlr ? <Th fit>Роуминг</Th> : null}
            {hlr ? <Th fit>Страна роуминга</Th> : null}
            {hlr ? <Th fit>Оператор роуминга</Th> : null}
            <Th fit>Ошибка</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map((it) => (
            <tr key={it.id}>
              <Td fit>
                <code>{it.phone}</code>
              </Td>
              <Td fit>
                <Badge tone={statusTone(it.status)}>{itemStatusLabel[it.status] ?? it.status}</Badge>
              </Td>
              <Td fit>{it.result_status ? resultLabel[it.result_status] ?? it.result_status : yn(it.is_reachable)}</Td>
              {hlr ? <Td fit>{dash(it.operator_name)}</Td> : null}
              {hlr ? <Td fit>{dash(it.country_code)}</Td> : null}
              {hlr ? <Td fit>{dash(it.region)}</Td> : null}
              {hlr ? <Td fit>{dash(it.mcc)}</Td> : null}
              {hlr ? <Td fit>{dash(it.mnc)}</Td> : null}
              {hlr ? <Td fit>{dash(it.imsi)}</Td> : null}
              {hlr ? <Td fit>{dash(it.msc)}</Td> : null}
              {hlr ? <Td fit>{yn(it.roaming)}</Td> : null}
              {hlr ? <Td fit>{dash(it.roaming_country)}</Td> : null}
              {hlr ? <Td fit>{dash(it.roaming_operator)}</Td> : null}
              <Td fit>{dash(it.error_message)}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <InfiniteSentinel disabled={!items.hasNextPage || items.isFetchingNextPage} onVisible={() => void items.fetchNextPage()} />
      {!items.isLoading && rows.length === 0 ? <EmptyState>Номеров пока нет</EmptyState> : null}
    </div>
  );
}
