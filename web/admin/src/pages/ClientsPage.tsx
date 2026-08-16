import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PAGE_SIZE, PageHeader, Pager, Table, Td, Th, statusTone, withPage, formatMoney } from "ui";
import { api, type ClientRow, type Settings } from "../api";

export function ClientsPage() {
  const qc = useQueryClient();
  const [status, setStatus] = useState("");
  const [offset, setOffset] = useState(0);
  const list = useQuery({
    queryKey: ["clients", status, offset],
    queryFn: () => api.get<{ items: ClientRow[] }>(withPage("/clients", offset, status ? { status } : {})),
  });
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [ownerName, setOwnerName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [ownerPassword, setOwnerPassword] = useState("");
  const create = useMutation({
    mutationFn: () =>
      api.post("/clients", { name, owner_name: ownerName, owner_email: ownerEmail, owner_password: ownerPassword }),
    onSuccess: () => {
      setOpen(false);
      setName("");
      setOwnerName("");
      setOwnerEmail("");
      setOwnerPassword("");
      setOffset(0);
      void qc.invalidateQueries({ queryKey: ["clients"] });
    },
  });
  const settings = useQuery({ queryKey: ["settings"], queryFn: () => api.get<Settings>("/settings") });
  const threshold = Number(settings.data?.low_balance_threshold ?? 100);
  const items = list.data?.items ?? [];

  return (
    <div>
      <PageHeader
        title="Клиенты"
        actions={
          <Button type="button" onClick={() => setOpen((v) => !v)}>
            Создать
          </Button>
        }
      />
      <div className="mb-3 flex gap-2 text-sm">
        {["", "active", "suspended"].map((s) => (
          <button
            key={s || "all"}
            type="button"
            className={`rounded-md px-2 py-1 ${status === s ? "bg-zinc-200" : "hover:bg-zinc-100"}`}
            onClick={() => {
              setStatus(s);
              setOffset(0);
            }}
          >
            {s === "" ? "все" : s === "active" ? "активные" : "приостановленные"}
          </button>
        ))}
      </div>
      {open ? (
        <Card className="mb-4">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate();
            }}
          >
            <Field label="Название">
              <Input value={name} onChange={(e) => setName(e.target.value)} required />
            </Field>
            <Field label="ФИО владельца">
              <Input value={ownerName} onChange={(e) => setOwnerName(e.target.value)} required />
            </Field>
            <Field label="Эл. почта владельца">
              <Input type="email" value={ownerEmail} onChange={(e) => setOwnerEmail(e.target.value)} required />
            </Field>
            <Field label="Пароль владельца (мин. 10)">
              <Input type="password" value={ownerPassword} onChange={(e) => setOwnerPassword(e.target.value)} minLength={10} required />
            </Field>
            {create.isError ? <ErrorBox error={create.error} /> : null}
            <Button type="submit" disabled={create.isPending}>
              Создать клиента
            </Button>
          </form>
        </Card>
      ) : null}
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Имя</Th>
            <Th>Статус</Th>
            <Th>Владелец</Th>
            <Th>Номера</Th>
            <Th>Баланс</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((c) => (
            <tr key={c.id}>
              <Td>
                <Link className="text-blue-700 hover:underline" to={`/clients/${c.id}`}>
                  {c.name}
                </Link>
              </Td>
              <Td>
                <Badge tone={statusTone(c.status)}>{c.status}</Badge>
              </Td>
              <Td>{c.owner_email}</Td>
              <Td>{c.assigned_count}</Td>
              <Td>
                {c.available_balance != null ? (
                  <Badge tone={Number(c.available_balance) < threshold ? "amber" : "green"}>
                    {formatMoney(c.available_balance, c.currency)}
                  </Badge>
                ) : (
                  "—"
                )}
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Клиентов нет</EmptyState> : null}
      <Pager offset={offset} limit={PAGE_SIZE} count={items.length} onChange={setOffset} />
    </div>
  );
}
