import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
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
import { api, type LookupJob } from "../api";
import { jobStatusLabel, lookupInflight, sourceLabel, typeLabel } from "../lookup";

export function LookupsPage() {
  const [checkType, setCheckType] = useState("");
  const [status, setStatus] = useState("");
  const list = useInfiniteQuery({
    queryKey: ["lookup-jobs", checkType, status],
    queryFn: ({ pageParam }) =>
      api.get<{ items: LookupJob[]; total: number }>(
        withPage("/lookups/jobs", pageParam, { check_type: checkType, status }, INFINITE_PAGE_SIZE),
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
      <PageHeader
        title="Проверки HLR / SSMS"
        actions={
          <div className="flex gap-2 text-sm">
            <Link className="text-blue-700 hover:underline" to="/hlr">
              HLR
            </Link>
            <Link className="text-blue-700 hover:underline" to="/silent-sms">
              Silent SMS
            </Link>
          </div>
        }
      />
      <div className="mb-3 grid gap-3 md:grid-cols-2">
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
            <Th fit>Создана</Th>
            <Th fit>Тип</Th>
            <Th fit>Источник</Th>
            <Th fit>Статус</Th>
            <Th fit>Всего</Th>
            <Th fit>В сети</Th>
            <Th fit>Не в сети</Th>
            <Th fit>Ошибки</Th>
            <Th fit>Списано</Th>
            <Th fit>Файл</Th>
            <Th fit>Ошибка</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((job) => (
            <tr key={job.id}>
              <Td fit>
                <Link className="text-blue-700 hover:underline" to={`/lookups/${job.id}`}>
                  {formatDateTime(job.created_at)}
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
              <Td fit>{job.original_filename || "—"}</Td>
              <Td fit>{job.error_message || "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <InfiniteSentinel disabled={!list.hasNextPage || list.isFetchingNextPage} onVisible={() => void list.fetchNextPage()} />
      {!list.isLoading && items.length === 0 ? <EmptyState>Проверок нет</EmptyState> : null}
    </div>
  );
}
