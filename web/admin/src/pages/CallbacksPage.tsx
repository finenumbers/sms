import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge, Card, EmptyState, ErrorBox, PAGE_SIZE, PageHeader, Pager, Table, Td, Th, withPage } from "ui";
import { api, type CallbackRow } from "../api";

export function CallbacksPage() {
  const { id } = useParams();
  const [offset, setOffset] = useState(0);
  const list = useQuery({
    queryKey: ["callbacks", offset],
    queryFn: () => api.get<{ items: CallbackRow[] }>(withPage("/callbacks", offset)),
    refetchInterval: 5000,
  });
  const detail = useQuery({
    queryKey: ["callback", id],
    queryFn: () => api.get<Record<string, unknown>>(`/callbacks/${id}`),
    enabled: Boolean(id),
  });
  const items = list.data?.items ?? [];

  return (
    <div>
      <PageHeader title="Колбэки" />
      <p className="mb-3 text-sm text-zinc-600">Сырые входящие события провайдера (доставка и входящие SMS). Показываем то, что пришло, без догадок о формате.</p>
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Время</Th>
            <Th>Тип</Th>
            <Th>Метод</Th>
            <Th>Размер</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((e) => (
            <tr key={e.id}>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/callbacks/${e.id}`}>
                  {e.created_at}
                </Link>
              </Td>
              <Td>
                <Badge>{e.kind}</Badge>
              </Td>
              <Td>{e.method}</Td>
              <Td>{e.body_bytes}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Пока нет входящих колбэков</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
      {id ? (
        <Card className="mt-4">
          <div className="mb-2 flex justify-between">
            <h2 className="font-medium">Сырые данные</h2>
            <Link className="text-sm text-blue-700" to="/callbacks">
              закрыть
            </Link>
          </div>
          {detail.isError ? <ErrorBox error={detail.error} /> : null}
          <pre className="max-h-[480px] overflow-auto rounded bg-zinc-950 p-3 text-xs text-zinc-100">
            {JSON.stringify(detail.data, null, 2)}
          </pre>
        </Card>
      ) : null}
    </div>
  );
}
