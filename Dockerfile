FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
COPY web/ui/package.json ./ui/
COPY web/admin/package.json ./admin/
COPY web/client/package.json ./client/
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/sms ./cmd/sms

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sms /sms
COPY --from=web /web/admin/dist /srv/admin
COPY --from=web /web/client/dist /srv/client
USER nonroot:nonroot
ENTRYPOINT ["/sms"]
CMD ["api"]
