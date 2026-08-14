import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Alert,
  Button,
  Card,
  Field,
  INFINITE_PAGE_SIZE,
  InfiniteSentinel,
  PageHeader,
  Table,
  Td,
  Textarea,
  Th,
  formatMoney,
  withPage,
} from "ui";
import { api, type LookupCheckType, type LookupEstimate, type LookupJob, type LookupPreview } from "../api";
import { lookupError, parsePhoneList, typeLabel } from "../lookup";

type PreviewPhone = { phone: string; line: number };

function PreviewPhonesTable({ previewID }: { previewID: string }) {
  const q = useInfiniteQuery({
    queryKey: ["lookup-preview-phones", previewID],
    queryFn: ({ pageParam }) =>
      api.get<{ items: PreviewPhone[]; total: number }>(
        withPage(`/lookups/csv-previews/${previewID}/phones`, pageParam, {}, INFINITE_PAGE_SIZE),
      ),
    initialPageParam: 0,
    getNextPageParam: (last, _pages, lastParam) => {
      const next = lastParam + last.items.length;
      return next < last.total ? next : undefined;
    },
  });
  const rows = q.data?.pages.flatMap((p) => p.items) ?? [];
  const total = q.data?.pages[0]?.total ?? 0;
  return (
    <div className="mb-4">
      <p className="mb-2 text-xs text-zinc-500">
        {total} номеров в файле
        {q.isFetchingNextPage ? " · загрузка…" : null}
      </p>
      <Table>
        <thead>
          <tr>
            <Th>№</Th>
            <Th>Номер</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={`${row.line}-${row.phone}`}>
              <Td>{row.line}</Td>
              <Td>
                <code>{row.phone}</code>
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <InfiniteSentinel disabled={!q.hasNextPage || q.isFetchingNextPage} onVisible={() => void q.fetchNextPage()} />
    </div>
  );
}

export function CheckCreatePage({ type }: { type: LookupCheckType }) {
  const nav = useNavigate();
  const qc = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const [list, setList] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<LookupPreview | null>(null);
  const [dragOver, setDragOver] = useState(false);

  const phones = parsePhoneList(list);
  const estimate = useQuery({
    queryKey: ["lookup-estimate", type, phones],
    queryFn: () =>
      api.post<LookupEstimate>(
        "/lookups/estimate",
        phones.length === 1 ? { type, phone: phones[0] } : { type, phones },
      ),
    enabled: !preview && phones.length > 0,
    retry: false,
  });
  const csvEstimate = useQuery({
    queryKey: ["lookup-csv-estimate", preview?.id],
    queryFn: () => api.post<LookupEstimate>(`/lookups/csv-previews/${preview!.id}/estimate`),
    enabled: Boolean(preview?.id),
    retry: false,
  });

  const upload = useMutation({
    mutationFn: (f: File) => api.upload<LookupPreview>("/lookups/csv-previews", f, "file", { type }),
    onSuccess: (row) => setPreview(row),
  });
  const submitSingle = useMutation({
    mutationFn: () => api.post<LookupJob>("/lookups/checks", { type, phone: phones[0] }),
    onSuccess: goJob,
  });
  const submitList = useMutation({
    mutationFn: () => api.post<LookupJob>("/lookups/jobs", { type, phones }),
    onSuccess: goJob,
  });
  const submitCSV = useMutation({
    mutationFn: () => api.post<LookupJob>(`/lookups/csv-previews/${preview!.id}/submit`),
    onSuccess: goJob,
  });

  function goJob(job: LookupJob) {
    void qc.invalidateQueries({ queryKey: ["lookup-jobs"] });
    void qc.invalidateQueries({ queryKey: ["balance"] });
    nav(`/lookups/${job.id}`);
  }

  function takeFile(next: File | null) {
    setFile(next);
    setPreview(null);
    if (next) {
      setList("");
      upload.mutate(next);
    }
  }

  const est = preview ? csvEstimate : estimate;
  const pending = submitSingle.isPending || submitList.isPending || submitCSV.isPending || upload.isPending;
  const actionError = submitSingle.error ?? submitList.error ?? submitCSV.error ?? upload.error ?? est.error;

  return (
    <div>
      <PageHeader
        title={`Проверка ${typeLabel[type]}`}
        actions={
          <Link className="text-sm text-blue-700 hover:underline" to="/lookups">
            История проверок
          </Link>
        }
      />
      {type === "hlr" ? (
        <p className="mb-3 text-sm text-zinc-600">Отправка HLR-запроса по номерам E.164.</p>
      ) : (
        <p className="mb-3 text-sm text-zinc-600">Отправка Silent SMS по номерам E.164.</p>
      )}
      <Card className="mb-4">
        {est.data ? (
          <p className="mb-3 text-sm text-zinc-600">
            Цена за единицу: {formatMoney(est.data.unit_sell_price, est.data.currency)}
          </p>
        ) : null}
        <Field label="CSV / TXT файл">
          <input
            ref={fileRef}
            type="file"
            accept=".csv,.txt,text/csv,text/plain"
            className="hidden"
            onChange={(e) => takeFile(e.target.files?.[0] ?? null)}
          />
          <button
            type="button"
            className={`w-full rounded-md border border-dashed px-3 py-8 text-center text-sm ${
              dragOver ? "border-blue-500 bg-blue-50" : "border-zinc-300 bg-zinc-50"
            }`}
            onClick={() => fileRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragOver(false);
              takeFile(e.dataTransfer.files[0] ?? null);
            }}
          >
            <span className="block font-medium text-zinc-800">Перетащите CSV/TXT сюда</span>
            <span className="mt-1 block text-xs text-zinc-500">
              Один номер на строку или в первом столбце. Файл разбирается сразу; проверка — после «Запустить проверку».
            </span>
            {file ? <span className="mt-2 block text-xs text-zinc-700">{file.name}</span> : null}
            {upload.isPending ? <span className="mt-2 block text-xs text-zinc-500">загрузка…</span> : null}
          </button>
          <Button type="button" variant="secondary" className="mt-2" onClick={() => fileRef.current?.click()}>
            Загрузить файл
          </Button>
        </Field>
        {preview ? <PreviewPhonesTable previewID={preview.id} /> : null}
        <p className="my-4 text-center text-xs text-zinc-500">или вставьте номера</p>
        <Field label="Номера (E.164, по одному в строке или через запятую)">
          <Textarea
            value={list}
            placeholder="+79991234567"
            rows={8}
            onChange={(e) => {
              setList(e.target.value);
              setPreview(null);
              setFile(null);
              if (fileRef.current) {
                fileRef.current.value = "";
              }
            }}
          />
        </Field>
        <p className="mb-3 text-xs text-zinc-500">{phones.length} номеров</p>
        {est.data ? (
          <p className="mb-3 text-sm text-zinc-600">
            {est.data.quantity} шт. × {formatMoney(est.data.unit_sell_price, est.data.currency)} ={" "}
            <span className="font-medium">{formatMoney(est.data.estimated_cost, est.data.currency)}</span>
            {preview ? ` · ${preview.phone_count} номеров в файле` : null}
          </p>
        ) : null}
        {actionError ? <Alert className="mb-3">{lookupError(actionError)}</Alert> : null}
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            disabled={pending || (!preview && phones.length === 0) || !est.data}
            onClick={() => {
              if (preview) {
                submitCSV.mutate();
              } else if (phones.length === 1) {
                submitSingle.mutate();
              } else {
                submitList.mutate();
              }
            }}
          >
            Запустить проверку
          </Button>
        </div>
      </Card>
      <p className="text-xs text-zinc-500">
        Пилот: только российские мобильные +79. Списание — после результата провайдера. Пока нет тарифа, кнопка покажет
        «услуга не подключена».
      </p>
    </div>
  );
}
