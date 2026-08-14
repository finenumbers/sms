import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Alert, Badge, Button, Card, EmptyState, Field, Input, PAGE_SIZE, PageHeader, Pager, Table, Td, Th, statusTone, withPage } from "ui";
import { api, type WebhookDelivery, type WebhookEndpoint } from "../api";
import { lookupError } from "../lookup";

const eventOptions = [
  { id: "check.completed", label: "проверка готова" },
  { id: "check.failed", label: "проверка с ошибкой" },
  { id: "job.completed", label: "задание завершено" },
];

export function WebhooksPage() {
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const [url, setUrl] = useState("");
  const [description, setDescription] = useState("");
  const [events, setEvents] = useState<string[]>([]);
  const [secret, setSecret] = useState<string | null>(null);
  const [openID, setOpenID] = useState<string | null>(null);

  const list = useQuery({
    queryKey: ["webhooks"],
    queryFn: () => api.get<{ items: WebhookEndpoint[] }>("/webhooks"),
  });
  const deliveries = useQuery({
    queryKey: ["webhook-deliveries", openID, offset],
    queryFn: () =>
      api.get<{ items: WebhookDelivery[] }>(withPage(`/webhooks/${openID}/deliveries`, offset)),
    enabled: Boolean(openID),
  });

  const create = useMutation({
    mutationFn: () => api.post<WebhookEndpoint>("/webhooks", { url, description: description || undefined, events }),
    onSuccess: (row) => {
      setUrl("");
      setDescription("");
      setEvents([]);
      setSecret(row.secret ?? null);
      void qc.invalidateQueries({ queryKey: ["webhooks"] });
    },
  });
  const patch = useMutation({
    mutationFn: (row: { id: string; enabled: boolean }) => api.patch<WebhookEndpoint>(`/webhooks/${row.id}`, { enabled: row.enabled }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["webhooks"] }),
  });
  const rotate = useMutation({
    mutationFn: (id: string) => api.post<WebhookEndpoint>(`/webhooks/${id}/rotate-secret`),
    onSuccess: (row) => {
      setSecret(row.secret ?? null);
      void qc.invalidateQueries({ queryKey: ["webhooks"] });
    },
  });
  const del = useMutation({
    mutationFn: (id: string) => api.delete(`/webhooks/${id}`),
    onSuccess: () => {
      setOpenID(null);
      void qc.invalidateQueries({ queryKey: ["webhooks"] });
    },
  });

  const items = list.data?.items ?? [];
  const logs = deliveries.data?.items ?? [];

  function toggleEvent(id: string) {
    setEvents((cur) => (cur.includes(id) ? cur.filter((e) => e !== id) : [...cur, id]));
  }

  return (
    <div>
      <PageHeader title="Webhooks" />
      <p className="mb-3 text-sm text-zinc-600">
        События проверок HLR и Silent SMS. Пустой список событий — все. Секрет показывается только при создании и смене.
      </p>
      <Card className="mb-4">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <Field label="URL">
            <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…" required />
          </Field>
          <Field label="Описание">
            <Input value={description} onChange={(e) => setDescription(e.target.value)} />
          </Field>
          <div className="mb-3 text-sm">
            <div className="mb-1 font-medium text-zinc-700">События</div>
            {eventOptions.map((ev) => (
              <label key={ev.id} className="mr-4 inline-flex items-center gap-1">
                <input type="checkbox" checked={events.includes(ev.id)} onChange={() => toggleEvent(ev.id)} />
                {ev.label}
              </label>
            ))}
            <p className="mt-1 text-xs text-zinc-500">Ничего не отмечено — подписка на все события.</p>
          </div>
          {create.isError ? <Alert className="mb-3">{lookupError(create.error)}</Alert> : null}
          <Button type="submit" disabled={create.isPending}>
            Добавить
          </Button>
        </form>
      </Card>
      {secret ? (
        <Alert tone="green" className="mb-4">
          Секрет (сохраните сейчас): <code className="break-all">{secret}</code>
        </Alert>
      ) : null}
      {list.isError ? <Alert className="mb-4">{lookupError(list.error)}</Alert> : null}
      <Table>
        <thead>
          <tr>
            <Th>URL</Th>
            <Th>События</Th>
            <Th>Статус</Th>
            <Th></Th>
          </tr>
        </thead>
        <tbody>
          {items.map((row) => (
            <tr key={row.id}>
              <Td>
                <div className="max-w-xs truncate" title={row.url}>
                  {row.url}
                </div>
                {row.description ? <div className="text-xs text-zinc-500">{row.description}</div> : null}
              </Td>
              <Td className="text-xs">{row.events.length === 0 ? "все" : row.events.join(", ")}</Td>
              <Td>
                <Badge tone={row.enabled ? "green" : "amber"}>{row.enabled ? "вкл" : "выкл"}</Badge>
                {row.consecutive_failures > 0 ? (
                  <div className="mt-1 text-xs text-zinc-500">неудач подряд: {row.consecutive_failures}</div>
                ) : null}
              </Td>
              <Td>
                <div className="flex flex-wrap gap-1">
                  <Button type="button" variant="secondary" onClick={() => patch.mutate({ id: row.id, enabled: !row.enabled })}>
                    {row.enabled ? "Выключить" : "Включить"}
                  </Button>
                  <Button type="button" variant="secondary" onClick={() => rotate.mutate(row.id)}>
                    Сменить секрет
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => {
                      setOpenID(row.id);
                      setOffset(0);
                    }}
                  >
                    Доставки
                  </Button>
                  <Button
                    type="button"
                    variant="danger"
                    onClick={() => {
                      if (window.confirm("Удалить webhook?")) {
                        del.mutate(row.id);
                      }
                    }}
                  >
                    Удалить
                  </Button>
                </div>
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Webhooks нет</EmptyState> : null}

      {openID ? (
        <div className="mt-6">
          <h2 className="mb-2 font-medium">Доставки</h2>
          {deliveries.isError ? <Alert className="mb-3">{lookupError(deliveries.error)}</Alert> : null}
          <Table>
            <thead>
              <tr>
                <Th>Время</Th>
                <Th>Событие</Th>
                <Th>Статус</Th>
                <Th>Попытки</Th>
                <Th>Ответ</Th>
              </tr>
            </thead>
            <tbody>
              {logs.map((d) => (
                <tr key={d.id}>
                  <Td>{d.created_at}</Td>
                  <Td>{d.event_type}</Td>
                  <Td>
                    <Badge tone={statusTone(d.status)}>{d.status}</Badge>
                  </Td>
                  <Td>{d.attempt_count}</Td>
                  <Td>{d.last_error ?? (d.last_response_code != null ? String(d.last_response_code) : "—")}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
          {!deliveries.isLoading && logs.length === 0 ? <EmptyState>Доставок нет</EmptyState> : null}
          <Pager offset={offset} limit={PAGE_SIZE} count={logs.length} onChange={setOffset} />
        </div>
      ) : null}
    </div>
  );
}
