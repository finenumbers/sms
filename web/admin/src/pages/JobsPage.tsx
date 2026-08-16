import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  Badge,
  EmptyState,
  ErrorBox,
  INFINITE_PAGE_SIZE,
  InfiniteSentinel,
  PageHeader,
  Select,
  Table,
  Td,
  Th,
  statusTone,
  withPage,
  formatDateTime,
  formatMoney,
} from "ui";
import { api, type ClientRow, type LookupJob } from "../api";
import { jobStatusLabel, lookupInflight, sourceLabel, typeLabel } from "../lookup";

export function JobsPage() {
  const [search] = useSearchParams();
  const [clientId, setClientId] = useState(search.get("client_id") ?? "");
  const [checkType, setCheckType] = useState(search.get("check_type") ?? "");
  const [status, setStatus] = useState(search.get("status") ?? "");
  const clients = useQuery({
    queryKey: ["clients", "jobs-filter"],
    queryFn: () => api.get<{ items: ClientRow[] }>(withPage("/clients", 0, {}, 100)),
  });
  const names = Object.fromEntries((clients.data?.items ?? []).map((c) => [c.id, c.name]));
  const list = useInfiniteQuery({
    queryKey: ["admin-lookup-jobs", clientId, checkType, status],
    queryFn: ({ pageParam }) =>
      api.get<{ items: LookupJob[]; total: number }>(
        withPage("/lookups/jobs", pageParam, { client_id: clientId, check_type: checkType, status }, INFINITE_PAGE_SIZE),
      ),
    initialPageParam: 0,
    getNextPageParam: (last, _pages, lastParam) => {
      const next = lastParam + last.items.length;
      return next < last.total ? next : undefined;
    },
    refetchInterval: (q) =>
      (q.state.data?.pages ?? []).some((p) => p.items.some((j) => lookupInflight(j.status))) ? 4000 : false,
  });
  const items = list.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <div>
      <PageHeader title="Задания" />
      <p className="mb-3 text-sm text-zinc-500">Проверки HLR и Silent SMS по всем клиентам.</p>
      <div className="mb-3 grid gap-3 md:grid-cols-3">
        <Select value={clientId} onChange={(e) => setClientId(e.target.value)}>
          <option value="">все клиенты</option>
          {(clients.data?.items ?? []).map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>
        <Select value={checkType} onChange={(e) => setCheckType(e.target.value)}>
          <option value="">все типы</option>
          <option value="hlr">HLR</option>
          <option value="ping">Silent SMS</option>
        </Select>
        <Select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">все статусы</option>
          <option value="queued">в очереди</option>
          <option value="processing">выполняется</option>
          <option value="completed">готово</option>
          <option value="completed_with_errors">готово с ошибками</option>
          <option value="failed">ошибка</option>
        </Select>
      </div>
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th fit>Создано</Th>
            <Th fit>Клиент</Th>
            <Th fit>Тип</Th>
            <Th fit>Источник</Th>
            <Th fit>Статус</Th>
            <Th fit>Номера</Th>
            <Th fit>В сети</Th>
            <Th fit>Не в сети</Th>
            <Th fit>Ошибки</Th>
            <Th fit>Списано</Th>
            <Th fit>Ошибка</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((job) => (
            <tr key={job.id}>
              <Td fit>
                <Link className="text-blue-700 hover:underline" to={`/jobs/${job.id}`}>
                  {formatDateTime(job.created_at)}
                </Link>
              </Td>
              <Td fit>
                <Link className="text-blue-700 hover:underline" to={`/clients/${job.client_id}`}>
                  {names[job.client_id] ?? job.client_id}
                </Link>
              </Td>
              <Td fit>{typeLabel[job.type] ?? job.type}</Td>
              <Td fit>{sourceLabel[job.source] ?? job.source}</Td>
              <Td fit>
                <Badge tone={statusTone(job.status)}>{jobStatusLabel[job.status] ?? job.status}</Badge>
              </Td>
              <Td fit>{job.item_count}</Td>
              <Td fit>{job.reachable_count ?? 0}</Td>
              <Td fit className={(job.unreachable_count ?? 0) > 0 ? "text-red-800" : undefined}>
                {job.unreachable_count ?? 0}
              </Td>
              <Td fit className={job.failure_count > 0 ? "text-red-800" : undefined}>
                {job.failure_count}
              </Td>
              <Td fit>{formatMoney(job.actual_cost || job.estimated_cost, job.currency)}</Td>
              <Td fit>{job.error_message || "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <InfiniteSentinel disabled={!list.hasNextPage || list.isFetchingNextPage} onVisible={() => void list.fetchNextPage()} />
      {!list.isLoading && items.length === 0 ? <EmptyState>Заданий нет</EmptyState> : null}
    </div>
  );
}
