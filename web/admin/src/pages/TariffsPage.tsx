import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge, Button, Card, EmptyState, ErrorBox, Field, Input, PageHeader, Select, Table, Td, Th } from "ui";
import { formatMoney } from "ui";
import { api, type TariffPlan } from "../api";
import { priceUnit, productLabel } from "../lookup";

const products = ["sms_domestic", "sms_international", "hlr", "silent_sms"] as const;

export function TariffsPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["tariffs"], queryFn: () => api.get<{ items: TariffPlan[] }>("/tariffs?limit=100") });
  const [open, setOpen] = useState(false);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [product, setProduct] = useState("sms_domestic");
  const [price, setPrice] = useState("");
  const create = useMutation({
    mutationFn: () =>
      api.post("/tariffs", {
        code,
        name,
        product,
        sell_price: price,
        currency: "RUB",
        is_active: true,
      }),
    onSuccess: () => {
      setOpen(false);
      setCode("");
      setName("");
      setPrice("");
      void qc.invalidateQueries({ queryKey: ["tariffs"] });
    },
  });
  const patch = useMutation({
    mutationFn: (row: TariffPlan & { sell_price: string; is_active: boolean }) =>
      api.patch(`/tariffs/${row.id}`, {
        name: row.name,
        sell_price: row.sell_price,
        is_default: row.is_default,
        is_active: row.is_active,
      }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["tariffs"] }),
  });
  const items = list.data?.items ?? [];
  return (
    <div>
      <PageHeader
        title="Тарифы"
        actions={
          <Button type="button" onClick={() => setOpen((v) => !v)}>
            Создать
          </Button>
        }
      />
      <p className="mb-3 text-sm text-zinc-500">
        SMS — цена за PDU, HLR и Silent SMS — за проверку. Себестоимость провайдера здесь не задаётся. Каталог клиентам сам не назначается.
      </p>
      {open ? (
        <Card className="mb-4">
          <Field label="Код">
            <Input value={code} onChange={(e) => setCode(e.target.value)} required />
          </Field>
          <Field label="Название">
            <Input value={name} onChange={(e) => setName(e.target.value)} required />
          </Field>
          <Field label="Продукт">
            <Select value={product} onChange={(e) => setProduct(e.target.value)}>
              {products.map((p) => (
                <option key={p} value={p}>
                  {productLabel[p]}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={`Цена ${priceUnit(product)}, RUB`}>
            <Input value={price} onChange={(e) => setPrice(e.target.value)} required />
          </Field>
          {create.isError ? <ErrorBox error={create.error} /> : null}
          <Button type="button" onClick={() => create.mutate()} disabled={create.isPending}>
            Сохранить
          </Button>
        </Card>
      ) : null}
      {list.isError ? <ErrorBox error={list.error} /> : null}
      <Table>
        <thead>
          <tr>
            <Th>Код</Th>
            <Th>Название</Th>
            <Th>Продукт</Th>
            <Th>Цена</Th>
            <Th>Статус</Th>
            <Th></Th>
          </tr>
        </thead>
        <tbody>
          {items.map((row) => (
            <TariffRow key={row.id} row={row} onSave={(next) => patch.mutate(next)} pending={patch.isPending} />
          ))}
        </tbody>
      </Table>
      {!list.isLoading && items.length === 0 ? <EmptyState>Тарифов нет</EmptyState> : null}
    </div>
  );
}

function TariffRow({
  row,
  onSave,
  pending,
}: {
  row: TariffPlan;
  onSave: (row: TariffPlan) => void;
  pending: boolean;
}) {
  const [name, setName] = useState(row.name);
  const [price, setPrice] = useState(row.sell_price);
  const [active, setActive] = useState(row.is_active);
  return (
    <tr>
      <Td>
        <code>{row.code}</code>
      </Td>
      <Td>
        <Input value={name} onChange={(e) => setName(e.target.value)} />
      </Td>
      <Td>{productLabel[row.product] ?? row.product}</Td>
      <Td>
        <Input value={price} onChange={(e) => setPrice(e.target.value)} />
        <div className="mt-1 text-xs text-zinc-500">
          {formatMoney(row.sell_price, row.currency)} {priceUnit(row.product)}
        </div>
      </Td>
      <Td>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
          {active ? <Badge tone="green">активен</Badge> : <Badge>выкл</Badge>}
        </label>
      </Td>
      <Td>
        <Button
          variant="secondary"
          type="button"
          disabled={pending}
          onClick={() => onSave({ ...row, name, sell_price: price, is_active: active })}
        >
          Сохранить
        </Button>
      </Td>
    </tr>
  );
}
