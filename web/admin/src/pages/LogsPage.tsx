import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PAGE_SIZE, PageHeader, Pager, Select, Table, Td, Th, withPage, formatDateTime } from "ui";
import { api, type OpsLogRow } from "../api";

const TABS: { id: string; label: string }[] = [
  { id: "", label: "Все" },
  { id: "http", label: "HTTP" },
  { id: "didapi", label: "DIDAPI" },
  { id: "queue", label: "Очереди" },
  { id: "ingress", label: "Колбэки" },
  { id: "audit", label: "Аудит" },
];

function levelTone(level: string): "zinc" | "blue" | "green" | "amber" | "red" {
  if (level === "error") return "red";
  if (level === "warn") return "amber";
  return "zinc";
}

export function LogsPage() {
  const { id } = useParams();
  const [offset, setOffset] = useState(0);
  const [category, setCategory] = useState("");
  const [level, setLevel] = useState("");
  const [q, setQ] = useState("");
  const [requestId, setRequestId] = useState("");
  const [appliedQ, setAppliedQ] = useState("");
  const [appliedRequestId, setAppliedRequestId] = useState("");
  const extra = useMemo(() => {
    const e: Record<string, string> = {};
    if (category) e.category = category;
    if (level) e.level = level;
    if (appliedQ.trim()) e.q = appliedQ.trim();
    if (appliedRequestId.trim()) e.request_id = appliedRequestId.trim();
    return e;
  }, [category, level, appliedQ, appliedRequestId]);
  const list = useQuery({
    queryKey: ["logs", offset, extra],
    queryFn: () => api.get<{ items: OpsLogRow[]; from: string; to: string }>(withPage("/logs", offset, extra)),
  });
  const detail = useQuery({
    queryKey: ["log", id],
    queryFn: () => api.get<OpsLogRow>(`/logs/${id}`),
    enabled: Boolean(id),
  });
  const items = list.data?.items ?? [];

  return (
    <div>
      <PageHeader title="Логи" />
      <p className="mb-3 text-sm text-zinc-600">
        Операционный журнал за последний час (окно не больше суток). Тела SMS, пароли и токены маскируются. В DIDAPI-событиях сохраняется запрос/ответ провайдера (для техподдержки). Сырые DLR/MO — на странице колбэков.
      </p>
      <div className="mb-3 flex flex-wrap gap-2">
        {TABS.map((tab) => (
          <Button
            key={tab.id || "all"}
            type="button"
            variant={category === tab.id ? "primary" : "secondary"}
            onClick={() => {
              setCategory(tab.id);
              setOffset(0);
            }}
          >
            {tab.label}
          </Button>
        ))}
      </div>
      <div className="mb-3 grid gap-3 md:grid-cols-4">
        <Field label="Уровень">
          <Select
            value={level}
            onChange={(e) => {
              setLevel(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">все</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </Select>
        </Field>
        <Field label="Поиск">
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="действие / кратко / ошибка" />
        </Field>
        <Field label="ID запроса">
          <Input value={requestId} onChange={(e) => setRequestId(e.target.value)} />
        </Field>
        <div className="flex items-end gap-2">
          <Button
            type="button"
            variant="secondary"
            onClick={() => {
              setAppliedQ(q);
              setAppliedRequestId(requestId);
              setOffset(0);
              void list.refetch();
            }}
            disabled={list.isFetching}
          >
            Обновить
          </Button>
        </div>
      </div>
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Время</Th>
            <Th>Кат.</Th>
            <Th>Ур.</Th>
            <Th>Действие</Th>
            <Th>HTTP</Th>
            <Th>Кратко</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((e) => (
            <tr key={e.id}>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/logs/${e.id}`}>
                  {formatDateTime(e.created_at)}
                </Link>
              </Td>
              <Td>
                <Badge>{e.category}</Badge>
              </Td>
              <Td>
                <Badge tone={levelTone(e.level)}>{e.level}</Badge>
              </Td>
              <Td className="font-mono text-xs">{e.action}</Td>
              <Td className="text-xs">
                {e.http_status ? `${e.http_method ?? ""} ${e.http_status}` : "—"}
              </Td>
              <Td className="max-w-md truncate text-xs text-zinc-600">{e.summary ?? e.error ?? "—"}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Нет событий за окно</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
      {id ? (
        <Card className="mt-4">
          <div className="mb-2 flex justify-between">
            <h2 className="font-medium">Детали</h2>
            <Link className="text-sm text-blue-700" to="/logs">
              закрыть
            </Link>
          </div>
          {detail.isError ? <ErrorBox error={detail.error} /> : null}
          {detail.data?.category === "ingress" && detail.data.resource_id ? (
            <p className="mb-2 text-sm">
              <Link className="text-blue-700 hover:underline" to={`/callbacks/${detail.data.resource_id}`}>
                Сырой колбэк
              </Link>
            </p>
          ) : null}
          <pre className="max-h-[480px] overflow-auto rounded bg-zinc-950 p-3 text-xs text-zinc-100">
            {JSON.stringify(detail.data, null, 2)}
          </pre>
        </Card>
      ) : null}
    </div>
  );
}
