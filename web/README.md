# Web (волна 8)

Два SPA: `admin` и `client`, общий `ui`. React + Vite + TanStack Query. В проде dist кладётся в образ; отдаёт процесс `api` по `Host`. Node в проде нет.

```bash
cd web
npm install
npm run dev:admin   # :5173, прокси /admin/v1 → :8080
npm run dev:client  # :5174, прокси /client/v1 → :8080
npm run build
```

Открывать dev-сервер с хоста `http://admin.sms.localhost:5173` / `http://client.sms.localhost:5174` (не `127.0.0.1`): cookie `__Host-` и CSRF (Host/Origin) должны совпасть с `ADMIN_HOST` / `CLIENT_HOST`. `*.localhost` — secure context, поэтому `COOKIE_SECURE=true` работает по HTTP. Vite не меняет `Host` на прокси (`changeOrigin: false`).
