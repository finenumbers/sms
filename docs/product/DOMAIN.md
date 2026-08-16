# Finenumbers SMS Service — Domain (v1 skeleton)

Black-box product domain for the first implementation wave.
Final DB schema / stack choices come in the architecture phase; this document freezes **roles, entities, and invariants**.

## Roles

| Role | How created | Capabilities (v1) |
|---|---|---|
| **Admin** | Seeded / managed internally | CRUD clients; import DEF numbers; assign/unassign numbers; system settings; API auth settings |
| **ClientUser** | Created by Admin (with Client) | Use client LK: send/receive SMS, campaigns, delivery confirmations |
| **Client API** | Credentials issued in Settings | Programmatic access mirroring LK SMS capabilities (details in architecture phase) |

**Invariant:** there is **no** public self-registration. Clients exist only after Admin creates them.

## Bounded contexts (logical)

1. **Identity & access** — AdminUser, Client, ClientUser, ApiCredential, sessions
2. **Number inventory** — DefNumber, NumberAssignment (history)
3. **Messaging** — SmsMessage, delivery state, inbound MO
4. **Campaigns** — SmsCampaign, campaign recipients / jobs (our logic, not Runexis)
5. **Platform settings** — SystemSettings (Runexis and SMSC credentials, callback base URL, lookup flag/limits)
6. **Runexis integration** — outbound DIDAPI client, token cache, callback ingress
7. **Billing** — prepaid wallet, tariff catalog (`sms_domestic` / `sms_international` per PDU; `hlr` / `silent_sms` per check), one ledger
8. **Lookup** — HLR and Silent SMS (Ping) via SMSC.ru; see [`LOOKUP.md`](LOOKUP.md)
9. **SMSC integration** — outbound adapter + signed callback (not Runexis)

## Entities

### AdminUser

Operator of the admin panel.

- `id`, `email`, `password_hash`, `name`, `status`, `created_at`, …

### Client

Tenant / customer organization.

- `id`, `name`, `status` (`active` / `suspended` / `deleted`), `purged_at` after hard wipe
- delete in admin wipes tenant SMS, campaigns, HLR/SSMS, wallet, users; numbers return to inventory; same email may be reused on a new client
- contact fields as needed later
- **no** Runexis credentials per client in v1 — platform uses one agent account

### ClientUser

Login principal for the client LK.

- `id`, `client_id`, `email`, `password_hash`, `name` (ФИО, required including the first owner), `role` (at least `owner`)
- minimum one owner per client at creation time; additional owners may be added by Admin

### DefNumber

Purchased DEF MSISDN tracked in our inventory (Admin loads already-purchased numbers from DIDAPI; not purchased via our UI).

- `id`, `msisdn` (canonical `7XXXXXXXXXX`)
- `status`: `inventory` | `assigned` | `disabled`
- optional metadata: region, notes, `supports_sms` flag, last Runexis sync snapshot
- unique on `msisdn`

### NumberAssignment

Binding of a number to a client (current + history).

- `id`, `def_number_id`, `client_id`
- `assigned_at`, `unassigned_at` (null = current)
- invariant: at most one open assignment per number

### SmsMessage

Normalized message record for LK history and delivery confirmation.

- `id` (our UUID)
- `client_id`
- `direction`: `outbound` | `inbound`
- `from_msisdn`, `to_msisdn`, `text`
- `provider` = `runexis`
- `provider_sms_id` (nullable until known — see GAPS)
- `campaign_id` (nullable)
- status machine (v1 target): `queued` → `accepted` → `sent` → `delivered` | `failed`
- timestamps: `created_at`, `accepted_at`, `sent_at`, `delivered_at`, `failed_at`
- raw provider payloads (callback / send response) for audit

### SmsCampaign

Group outbound SMS (implemented by us).

- `id`, `client_id`, `from_msisdn`, `text`
- `status`: `draft` | `queued` | `running` | `completed` | `failed` | `cancelled`
- recipient source (list upload / explicit MSISDNs)
- counters: total / accepted / delivered / failed
- workers fan-out to `SmsMessage` + `POST /api/v1/sms/send`

### ApiCredential

Client API authorization material managed under Settings.

- `id`, `client_id`, `name`, `key_prefix`, `secret_hash`
- `scopes` / status / last used
- plaintext secret shown once at creation

### SystemSettings

Platform-wide configuration (admin Settings section).

- Runexis agent `email` / secret reference (not plaintext in logs)
- callback public base URL for global `dlr-url` / `hook-url`
- default SMS direction policy for newly managed numbers
- rate limits / retention knobs (later)

## Core invariants

1. Client can send/receive only on **currently assigned** DEF numbers.
2. Unassigning a number immediately revokes LK/API use of that MSISDN (soft history retained).
3. Outbound product SMS always goes through Runexis `POST /api/v1/sms/send` (not SIM informational SMS).
4. Campaigns never call a non-existent bulk DIDAPI — they are N single sends under our job runner.
5. Admin is the only party that can create clients and mutate inventory assignment.

## Admin panel (v1 surface)

- Clients: create / update / delete (wipes tenant data; tombstone stays `deleted`)
- DEF numbers: load purchased numbers from DIDAPI; assign / unassign to client
- Settings: system config + API authorization management
- Extension points reserved for later modules
- Lookup: Jobs explorer, Monitoring (SMSC balance / cost probe / connectivity)

## Client LK (v1 surface)

- Send SMS
- Inbox (incoming SMS)
- Group SMS campaigns
- Delivery confirmations / message status history
- Extension points reserved for later modules
- HLR check, Silent SMS check, lookup history, lookup webhooks

## Non-goals (v1)

- Buying / booking numbers via DIDAPI
- Public registration
- Per-client Runexis agent accounts
- Full telecom CRM (agreements, MNP, labels, MAV, …)
