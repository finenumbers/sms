export class ApiError extends Error {
  status: number;
  code: string;
  rejectedPhones?: string[];

  constructor(status: number, code: string, message: string, rejectedPhones?: string[]) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.rejectedPhones = rejectedPhones;
  }
}

const errorByCode: Record<string, string> = {
  unauthorized: "Нужна авторизация",
  forbidden: "Недостаточно прав",
  invalid_json: "Некорректное тело запроса",
  not_assigned: "Номер не назначен этому клиенту",
  int_out_disabled: "Международные SMS выключены",
  rate_limited: "Слишком много запросов, подождите",
  not_found: "Не найдено",
  unavailable: "Сервис временно недоступен",
  conflict: "Конфликт с текущим состоянием",
  insufficient_funds: "Недостаточно средств",
  tariff_not_configured: "Тариф не назначен",
  lookup_disabled: "Услуга не подключена",
  client_suspended: "Клиент заблокирован",
  invalid_tariff: "Тариф недействителен",
  negative_balance_forbidden: "Отрицательный баланс запрещён",
  email_taken: "Email уже занят",
};

const validationByMessage: Record<string, string> = {
  "name required": "Укажите название клиента",
  "owner name required": "Укажите ФИО владельца",
  "owner name too long": "ФИО слишком длинное",
  "email required": "Укажите email",
  "invalid email": "Некорректный email",
  "password must be at least 10 characters": "Пароль не короче 10 символов",
};

function validationText(message: string): string {
  const raw = message.replace(/^validation:\s*/i, "").trim();
  return validationByMessage[raw] ?? (raw || "Проверьте поля запроса");
}

export function formatApiError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "validation") {
      return validationText(error.message);
    }
    if (errorByCode[error.code]) {
      return errorByCode[error.code];
    }
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "ошибка";
}

type Json = Record<string, unknown> | unknown[] | string | number | boolean | null;

async function parseBody(res: Response): Promise<unknown> {
  if (res.status === 204) {
    return undefined;
  }
  const text = await res.text();
  if (!text) {
    return undefined;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("X-Requested-With", "XMLHttpRequest");
  const isForm = typeof FormData !== "undefined" && init.body instanceof FormData;
  if (init.body && !isForm && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, credentials: "include", headers });
  const data = await parseBody(res);
  if (!res.ok) {
    const err = (data ?? {}) as { error?: { code?: string; message?: string; rejected_phones?: unknown } };
    const rejected = Array.isArray(err.error?.rejected_phones)
      ? err.error.rejected_phones.filter((p): p is string => typeof p === "string")
      : undefined;
    throw new ApiError(
      res.status,
      err.error?.code ?? "error",
      err.error?.message ?? res.statusText,
      rejected?.length ? rejected : undefined,
    );
  }
  return data as T;
}

export function createApi(base: string) {
  const url = (path: string) => (path.startsWith("http") ? path : `${base}${path}`);
  return {
    get: <T>(path: string) => request<T>(url(path)),
    post: <T>(path: string, body?: Json) =>
      request<T>(url(path), { method: "POST", body: body === undefined ? undefined : JSON.stringify(body) }),
    patch: <T>(path: string, body: Json) =>
      request<T>(url(path), { method: "PATCH", body: JSON.stringify(body) }),
    delete: (path: string) => request<void>(url(path), { method: "DELETE" }),
    upload: <T>(path: string, file: File, field = "file", extra?: Record<string, string>) => {
      const fd = new FormData();
      if (extra) {
        for (const [k, v] of Object.entries(extra)) {
          fd.set(k, v);
        }
      }
      fd.set(field, file);
      return request<T>(url(path), { method: "POST", body: fd });
    },
  };
}

export type Api = ReturnType<typeof createApi>;
