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
import { jobStatusLabel, lookupInflight, typeLabel } from "../lookup";

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
        title="Проверки"
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
            <Th>Создана</Th>
            <Th>Тип</Th>
            <Th>Статус</Th>
            <Th>Номеров</Th>
            <Th>Оценка</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((job) => (
            <tr key={job.id}>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/lookups/${job.id}`}>
                  {formatDateTime(job.created_at)}
                </Link>
              </Td>
              <Td>{typeLabel[job.type] ?? job.type}</Td>
              <Td>
                <Badge tone={statusTone(job.status)}>{jobStatusLabel[job.status] ?? job.status}</Badge>
              </Td>
              <Td>
                {job.success_count + job.failure_count}/{job.item_count}
              </Td>
              <Td>{formatMoney(job.estimated_cost, job.currency)}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <InfiniteSentinel disabled={!list.hasNextPage || list.isFetchingNextPage} onVisible={() => void list.fetchNextPage()} />
      {!list.isLoading && items.length === 0 ? <EmptyState>Проверок нет</EmptyState> : null}
    </div>
  );
}
