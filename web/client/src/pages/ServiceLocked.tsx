import type { ReactNode } from "react";
import { Card, PageHeader } from "ui";
import { useClientProducts } from "../tariffs";

export function ServiceLocked({ title }: { title: string }) {
  return (
    <div>
      <PageHeader title={title} />
      <Card>
        <p className="text-sm text-zinc-700">
          Услуга не подключена. Чтобы пользоваться этим разделом, подключите тариф на данную услугу у Департамента
          продаж.
        </p>
      </Card>
    </div>
  );
}

export function RequireProduct({
  anyOf,
  title,
  children,
}: {
  anyOf: readonly string[];
  title: string;
  children: ReactNode;
}) {
  const { ready, has } = useClientProducts();
  if (!ready) {
    return <div className="text-sm text-zinc-500">Загрузка…</div>;
  }
  if (!has(...anyOf)) {
    return <ServiceLocked title={title} />;
  }
  return <>{children}</>;
}
