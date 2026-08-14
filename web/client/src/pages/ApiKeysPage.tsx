import { useQuery } from "@tanstack/react-query";
import { Badge, EmptyState, ErrorBox, PageHeader, Table, Td, Th, statusTone } from "ui";
import { api, type APIKey } from "../api";

export function ApiKeysPage() {
  const q = useQuery({
    queryKey: ["apikeys"],
    queryFn: () => api.get<{ items: APIKey[] }>("/api-keys"),
  });
  return (
    <div>
      <PageHeader title="API-ключи" />
      <p className="mb-3 text-sm text-zinc-600">Ключи выдаёт администратор. Здесь только префикс, статус и время последнего использования.</p>
      {q.isError ? <ErrorBox error={q.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Имя</Th>
            <Th>Префикс</Th>
            <Th>Статус</Th>
            <Th>Последнее использование</Th>
          </tr>
        </thead>
        <tbody>
          {(q.data?.items ?? []).map((k) => (
            <tr key={k.id}>
              <Td>{k.name}</Td>
              <Td>
                <code>{k.key_prefix}</code>
              </Td>
              <Td>
                <Badge tone={statusTone(k.status)}>{k.status}</Badge>
              </Td>
              <Td>{k.last_used_at ?? "—"}</Td>
            </tr>
            ))}
          </tbody>
        </Table>
        {!q.isLoading && (q.data?.items ?? []).length === 0 ? <EmptyState>Ключей нет</EmptyState> : null}
    </div>
  );
}
