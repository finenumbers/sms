import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PAGE_SIZE, PageHeader, Pager, Select, Table, Td, Th, statusTone, withPage } from "ui";
import { api, type ClientRow, type NumberRow } from "../api";

type SyncReport = {
  fetched: number;
  sms_ok: number;
  imported: number;
  updated: number;
  skipped_no_sms: number;
  skipped_invalid: number;
  truncated: boolean;
  errors: string[];
};

export function NumbersPage() {
  const qc = useQueryClient();
  const [status, setStatus] = useState("");
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const list = useQuery({
    queryKey: ["numbers", status, q, offset],
    queryFn: () => api.get<{ items: NumberRow[] }>(withPage("/numbers", offset, { status, q })),
  });
  const clients = useQuery({
    queryKey: ["clients", "active"],
    queryFn: () => api.get<{ items: ClientRow[] }>(withPage("/clients", 0, { status: "active" }, 100)),
  });
  const [assignId, setAssignId] = useState<string | null>(null);
  const [editId, setEditId] = useState<string | null>(null);
  const [clientId, setClientId] = useState("");
  const [region, setRegion] = useState("");
  const [notes, setNotes] = useState("");
  const [report, setReport] = useState<SyncReport | null>(null);

  const sync = useMutation({
    mutationFn: () => api.post<SyncReport>("/numbers/sync"),
    onSuccess: (r) => {
      setReport(r);
      setOffset(0);
      void qc.invalidateQueries({ queryKey: ["numbers"] });
    },
  });
  const assign = useMutation({
    mutationFn: () => api.post(`/numbers/${assignId}/assign`, { client_id: clientId }),
    onSuccess: () => {
      setAssignId(null);
      void qc.invalidateQueries({ queryKey: ["numbers"] });
    },
  });
  const unassign = useMutation({
    mutationFn: (id: string) => api.post(`/numbers/${id}/unassign`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["numbers"] }),
  });
  const disable = useMutation({
    mutationFn: (id: string) => api.patch(`/numbers/${id}`, { status: "disabled" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["numbers"] }),
  });
  const enable = useMutation({
    mutationFn: (id: string) => api.patch(`/numbers/${id}`, { status: "inventory" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["numbers"] }),
  });
  const saveMeta = useMutation({
    mutationFn: () => api.patch(`/numbers/${editId}`, { region, notes }),
    onSuccess: () => {
      setEditId(null);
      void qc.invalidateQueries({ queryKey: ["numbers"] });
    },
  });
  const items = list.data?.items ?? [];

  return (
    <div>
      <PageHeader
        title="DEF-номера"
        actions={
          <Button type="button" disabled={sync.isPending} onClick={() => sync.mutate()}>
            {sync.isPending ? "Загрузка…" : "Загрузить данные"}
          </Button>
        }
      />
      <p className="mb-3 text-sm text-zinc-500">
        Загрузка берёт уже купленные номера из DIDAPI. Ответ 200 на sms/account значит «есть SMS-ресурс», но не гарантирует, что отправка пройдёт.
      </p>
      <div className="mb-3 flex flex-wrap items-end gap-3">
        <Field label="Поиск">
          <Input
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setOffset(0);
            }}
            placeholder="7…"
          />
        </Field>
        <Field label="Статус">
          <Select
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">все</option>
            <option value="inventory">в пуле</option>
            <option value="assigned">назначен</option>
            <option value="disabled">отключён</option>
          </Select>
        </Field>
      </div>
      {report ? (
        <p className="mb-3 text-sm text-zinc-600">
          получено {report.fetched}, SMS {report.sms_ok}, новых {report.imported}, обновлено {report.updated},
          без SMS {report.skipped_no_sms}, некорректных {report.skipped_invalid}
          {report.truncated ? ", список обрезан" : ""}
        </p>
      ) : null}
      {report?.errors?.length ? (
        <ul className="mb-3 max-h-40 overflow-auto text-xs text-red-700">
          {report.errors.slice(0, 50).map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ul>
      ) : null}
      {sync.isError ? <ErrorBox error={sync.error} /> : null}
      {list.isError ? <ErrorBox error={list.error} /> : null}
      {assignId ? (
        <Card className="mb-3">
          <Field label="Назначить клиенту">
            <Select value={clientId} onChange={(e) => setClientId(e.target.value)}>
              <option value="">выберите</option>
              {(clients.data?.items ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          </Field>
          {assign.isError ? <ErrorBox error={assign.error} /> : null}
          <div className="flex gap-2">
            <Button type="button" disabled={!clientId || assign.isPending} onClick={() => assign.mutate()}>
              Назначить
            </Button>
            <Button variant="ghost" type="button" onClick={() => setAssignId(null)}>
              Отмена
            </Button>
          </div>
        </Card>
      ) : null}
      {editId ? (
        <Card className="mb-3">
          <Field label="Регион">
            <Input value={region} onChange={(e) => setRegion(e.target.value)} />
          </Field>
          <Field label="Заметки">
            <Input value={notes} onChange={(e) => setNotes(e.target.value)} />
          </Field>
          {saveMeta.isError ? <ErrorBox error={saveMeta.error} /> : null}
          <div className="flex gap-2">
            <Button type="button" disabled={saveMeta.isPending} onClick={() => saveMeta.mutate()}>
              Сохранить
            </Button>
            <Button variant="ghost" type="button" onClick={() => setEditId(null)}>
              Отмена
            </Button>
          </div>
        </Card>
      ) : null}
      <Table>
        <thead>
          <tr>
            <Th>Номер</Th>
            <Th>Статус</Th>
            <Th>Клиент</Th>
            <Th>Регион</Th>
            <Th></Th>
          </tr>
        </thead>
        <tbody>
          {items.map((n) => (
            <tr key={n.id}>
              <Td>
                <code>{n.msisdn}</code>
              </Td>
              <Td>
                <Badge tone={statusTone(n.status)}>{n.status}</Badge>
              </Td>
              <Td>{n.client_name ?? "—"}</Td>
              <Td>{n.region ?? "—"}</Td>
              <Td>
                <div className="flex flex-wrap gap-1">
                  <Button
                    variant="ghost"
                    type="button"
                    onClick={() => {
                      setEditId(n.id);
                      setRegion(n.region ?? "");
                      setNotes(n.notes ?? "");
                    }}
                  >
                    Регион и заметки
                  </Button>
                  {n.status === "inventory" ? (
                    <Button variant="ghost" type="button" onClick={() => setAssignId(n.id)}>
                      Назначить
                    </Button>
                  ) : null}
                  {n.status === "assigned" ? (
                    <Button variant="ghost" type="button" onClick={() => unassign.mutate(n.id)}>
                      Снять
                    </Button>
                  ) : null}
                  {n.status === "inventory" ? (
                    <Button variant="ghost" type="button" onClick={() => disable.mutate(n.id)}>
                      Отключить
                    </Button>
                  ) : null}
                  {n.status === "disabled" ? (
                    <Button variant="ghost" type="button" onClick={() => enable.mutate(n.id)}>
                      В пул
                    </Button>
                  ) : null}
                </div>
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Номеров нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
      {unassign.isError ? <div className="mt-2"><ErrorBox error={unassign.error} /></div> : null}
      {disable.isError ? <div className="mt-2"><ErrorBox error={disable.error} /></div> : null}
    </div>
  );
}
