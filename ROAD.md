# TechNova — Dokumentasi Pengerjaan (Rewrite dari Nol)

> Helpdesk gateway multi-tenant: **Web (SIMRS) ⟷ Go ⟷ Telegram Forum Topic**, dengan AI sebagai first-responder.
> Dokumen ini adalah panduan kerja: keputusan final, urutan pengerjaan, kontrak API, dan kriteria selesai tiap fase.
>
> Terakhir diperbarui: 2026-10-09

---

## Daftar Isi
1. [Ringkasan](#1-ringkasan)
2. [Status Saat Ini](#2-status-saat-ini)
3. [Keputusan Final](#3-keputusan-final)
4. [Struktur Project](#4-struktur-project)
5. [Alur Sistem](#5-alur-sistem)
6. [State Machine Tiket](#6-state-machine-tiket)
7. [Aturan Routing Pesan](#7-aturan-routing-pesan)
8. [Kontrak API & WebSocket](#8-kontrak-api--websocket)
9. [Roadmap Pengerjaan (Fase 0–10)](#9-roadmap-pengerjaan)
10. [Checklist Keamanan](#10-checklist-keamanan)
11. [Strategi Testing](#11-strategi-testing)
12. [Skenario Uji End-to-End](#12-skenario-uji-end-to-end-acceptance)
13. [Deploy & Operasional](#13-deploy--operasional)
14. [Konvensi Kerja](#14-konvensi-kerja)
15. [Risiko & Catatan](#15-risiko--catatan)

---

## 1. Ringkasan

Alur inti yang dibangun:

1. Web SIMRS (PHP/JS) terhubung ke Go lewat REST + WebSocket.
2. User mengetik kendala → **AI membalas** di layar user.
3. Pada pesan pertama, Go mengirim **kartu** ke **General** supergroup Telegram: nama RS, nama user, modul, ringkasan singkat masalah, tombol **Klaim Tiket**.
4. Programmer menekan tombol → Go membuat **Forum Topic** khusus tiket itu, AI dimatikan untuk tiket tersebut.
5. Chat berjalan dua arah **hanya** antara web user ⟷ topik tiket itu. Chat di General **tidak pernah** diteruskan ke user mana pun.
6. Programmer mengetik `/close` atau `/selesai` → tiket selesai → user memberi rating CSAT 1–5.

---

## 2. Status Saat Ini

- [x] Keputusan arsitektur dan skema database final
- [x] Migrasi final `migrations/000001_create_schema` (up + down)
- [x] `internal/config/config.go` + `.env.example` *(belum dikompilasi — jalankan `gofmt` dan `go vet`)*
- [x] Backup kode lama sudah di-push ke GitHub (`main` @ `c39deeb`)
- [ ] Hapus file lama langsung di `main` + tag `backup-sebelum-rewrite`, siapkan folder (lihat Fase 0)
- [ ] Revoke token bot Telegram lama (token sempat tertulis di README lama)
- [ ] Fase 1–10 (lihat roadmap)

---

## 3. Keputusan Final

Keputusan di bawah **tidak dibahas ulang** selama pengerjaan, kecuali ada masalah nyata.

| Topik | Keputusan |
|---|---|
| Bahasa & framework | Go, Gin, GORM, PostgreSQL 13+, Gorilla WebSocket |
| Struktur | Package per fitur (`tenant`, `ticket`, `chat`, ...), alur `handler → service → repository` |
| Tenant | Didaftarkan **manual** lewat CLI `tenantctl`. Tidak ada auto-provision |
| Identitas tenant | Dari **API key** (header `X-API-Key`), disimpan sebagai hash SHA-256. `app_name` dan `tenant_name` diambil dari DB, bukan dari request |
| Siapa memanggil `init` | **Server PHP** KlikMedic (server-to-server), bukan browser. API key tidak pernah ada di JS |
| Auth browser | **Token tiket** (JWT HS256, `github.com/golang-jwt/jwt/v5`), berisi `public_id` tiket, `tenant_id`, `user_id`, `exp` |
| ID di API | `public_id` (UUID). ID integer hanya untuk internal. Tiket ditentukan dari token, bukan dari body/path |
| `user_id` | **Wajib**. Satu tiket aktif per user per tenant (dijaga unique index) |
| Status tiket | `open` → `escalated` → `resolved` (`open` boleh langsung `resolved`) |
| Kartu General | Dikirim **sekali**, saat pesan user pertama. Ringkasan dipotong 150 karakter. Diedit hanya saat diklaim |
| AI | Membalas selama status `open`. Diam total setelah `escalated`. Jalan **async**, tidak memblokir request |
| Lampiran | Disimpan sebagai `attachment_path` relatif. Disajikan lewat **signed URL** berumur pendek, bukan folder publik |
| Tipe upload | Gambar: jpeg/png/webp/gif. Video: mp4/webm. Dicek dari isi file (MIME sniffing), bukan ekstensi |
| Konten ke Telegram | Selalu di-**HTML-escape** (`html.EscapeString`) sebelum dikirim dengan `parse_mode=HTML` |
| Webhook Telegram | Wajib `secret_token`, cek `chat.id`, dedup `update_id`, balas 200 dulu lalu proses async |
| Pengiriman gagal | Pesan user disimpan dengan `delivery_status=pending`; worker outbox mengulang |
| Auto-close | `open` tanpa pesan user > 12 jam, `escalated` tanpa aktivitas > 72 jam → `resolved` |
| Logging | `log/slog` format JSON. Jangan pernah log token bot, URL Telegram penuh, atau query string WS |
| Realtime | Hub dengan satu goroutine pemilik state. **Satu event = satu frame WebSocket** |
| CORS & identitas RS | RS dikenali dari **API key**, bukan IP/domain. CORS `*` (aman karena auth pakai bearer token, bukan cookie). RS boleh pakai IP lokal/publik tanpa domain; semua koneksi RS → VPS bersifat **keluar** (outbound HTTPS/WSS) |

---

## 4. Struktur Project

```
tech-nova/
├── cmd/
│   ├── server/main.go          # wiring + graceful shutdown
│   └── tenantctl/main.go       # CLI daftar tenant & generate API key
├── internal/
│   ├── config/                 # env + validasi
│   ├── database/               # koneksi GORM (sudah ada)
│   ├── auth/                   # hash API key, token tiket, middleware, cek webhook secret, CORS
│   ├── tenant/                 # model, repository
│   ├── ticket/                 # model, dto, repository, service, handler
│   ├── chat/                   # model, dto, repository, service, handler (+ upload)
│   ├── ai/                     # client LLM
│   ├── telegram/               # client Bot API, webhook handler, dto
│   ├── realtime/               # hub, client, handler WebSocket
│   └── storage/                # validasi, penyimpanan & penyajian file
├── migrations/                 # 000001_create_schema.{up,down}.sql
├── scripts/migrate.sh
├── web/                        # index.html (demo) + assets/widget.js
├── docs/PENGERJAAN.md
└── .env.example
```

**Aturan dependensi (supaya tidak ada import melingkar dan mudah di-test):**

- `handler → service → repository`; handler tidak menyentuh repository.
- `ticket` dan `chat` **tidak meng-import `telegram`**. Mereka mendefinisikan interface `Notifier` yang dibutuhkan, `telegram` yang mengimplementasikannya, dan `main.go` yang menyambungkan.
- `realtime` tidak tahu apa pun tentang domain; hanya menerima `Publish(roomKey, event)`.
- `storage` berdiri sendiri (tidak meng-import package domain).

Sketsa interface (didefinisikan di sisi pemakai):

```go
// package chat
type Notifier interface {
    NotifyNewTicket(ctx context.Context, t TicketInfo, summary string) (cardMessageID int64, err error)
    SendToTopic(ctx context.Context, threadID int64, m OutboundMessage) (telegramMessageID int64, err error)
}

type Publisher interface { // diimplementasikan realtime.Hub
    Publish(roomKey string, event string, data any)
}
```

---

## 5. Alur Sistem

```
┌───────────────┐   init (X-API-Key)    ┌──────────┐
│ Server PHP RS │ ────────────────────► │          │
└──────┬────────┘ ◄──── token tiket ─── │          │
       │ (token dikirim ke halaman)     │          │
┌──────▼────────┐  REST + WS (Bearer)   │   Go     │ ──► AI (OpenRouter)
│ Widget (JS)   │ ◄───────────────────► │ TechNova │
└───────────────┘                       │          │ ──► Telegram Bot API
                                        │          │ ◄── Webhook (secret_token)
                                        └────┬─────┘
                                             │
                                        ┌────▼─────┐
                                        │ Postgres │
                                        └──────────┘
```

**Urutan kejadian satu tiket:**

1. `init` → buat/ambil tiket aktif user, kembalikan token.
2. Pesan user #1 → simpan → siarkan ke WS → kirim **kartu** ke General (sekali) → AI menjawab (async).
3. Pesan user berikutnya (masih `open`) → simpan → AI menjawab. Kartu **tidak** dikirim ulang.
4. Programmer klik **Klaim** → UPDATE atomik → `createForumTopic` → kirim sapaan + replay riwayat → edit kartu (tombol hilang) → event `ticket_claimed` ke WS.
5. Chat dua arah web ⟷ topik. AI diam.
6. `/close` atau `/selesai` di topik → `resolved` → event `ticket_resolved` → user mengisi rating → ringkasan rating dikirim ke topik → topik ditutup (`closeForumTopic`).

---

## 6. State Machine Tiket

| Dari | Ke | Pemicu | Efek di DB | Efek samping |
|---|---|---|---|---|
| — | `open` | `init` (tidak ada tiket aktif) | insert tiket | token diterbitkan |
| `open` | `escalated` | Klik **Klaim** di Telegram | `status`, `assigned_programmer`, `assigned_tg_user_id`, `claimed_at` diisi dalam **satu** UPDATE, syarat `status='open'` | buat topik, edit kartu, event WS |
| `escalated` | `open` | Pembuatan topik **gagal** (kompensasi) | kosongkan field klaim, syarat `telegram_thread_id IS NULL` | jawab callback dengan pesan gagal, tombol tetap aktif |
| `open` / `escalated` | `resolved` | `/close`, `/selesai`, tombol selesai dari user, atau auto-close | `status`, `resolved_at` | event WS, tutup topik |
| `resolved` | — | final | rating boleh diisi **sekali** | ringkasan rating ke topik |

SQL klaim (atomik, kunci anti double-claim):

```sql
UPDATE tickets
SET status = 'escalated',
    assigned_programmer = $1,
    assigned_tg_user_id = $2,
    claimed_at = now()
WHERE id = $3 AND status = 'open'
RETURNING *;
-- RowsAffected = 0  →  sudah diklaim orang lain
```

> Constraint database memaksa: `escalated` wajib punya `assigned_programmer` dan `claimed_at`; `resolved` wajib punya `resolved_at`.

---

## 7. Aturan Routing Pesan

**Telegram → Web** (webhook):

| Kondisi pesan | Aksi |
|---|---|
| Dari bot (`from.is_bot`) | Abaikan |
| Tanpa `message_thread_id` (General) | **Abaikan. Tidak pernah diteruskan ke user** |
| Topik tidak cocok dengan tiket aktif | Abaikan |
| Command `/close` atau `/selesai` (termasuk `/close@NamaBot`, deteksi lewat `entities`) | Selesaikan tiket |
| Teks / foto / video di topik tiket aktif | Simpan sebagai `programmer`, siarkan ke WS tiket tersebut |
| Siapa pun yang menulis di topik itu | Diteruskan (tidak harus yang mengklaim) |

**Web → Telegram:**

| Status tiket | Aksi |
|---|---|
| `open` | Simpan, AI menjawab. Kartu ke General hanya pada pesan pertama |
| `escalated` | Simpan (`delivery_status=pending`), kirim ke topik, tandai `sent` jika berhasil |
| `resolved` | Tolak dengan `409 Conflict` |

---

## 8. Kontrak API & WebSocket

Semua response sukses dibungkus `{"status": true, ...}`, error `{"status": false, "message": "..."}`.

### Server-to-server (dipanggil dari PHP, header `X-API-Key`)

**`POST /api/v1/tickets/init`**

```json
// Request
{
  "user_id": "205",
  "user_name": "Fajar (Admin Rawat Inap)",
  "module_name": "Rawat Inap",
  "diagnostic_info": { "url": "/rawat-inap/tindakan", "js_error": null }
}

// Response 200
{
  "status": true,
  "token": "<jwt>",
  "expires_at": "2026-10-10T08:00:00Z",
  "ticket": {
    "public_id": "7b1f...-uuid",
    "ticket_code": "TCK-20261009-b67be0",
    "status": "open",
    "module_name": "Rawat Inap",
    "assigned_programmer": null,
    "created_at": "2026-10-09T08:00:00Z"
  }
}
```

### Browser (header `Authorization: Bearer <token>`)

| Method | Path | Fungsi |
|---|---|---|
| `GET` | `/api/v1/ticket` | Ambil tiket milik token |
| `GET` | `/api/v1/ticket/messages?after_id=0&limit=50` | Riwayat chat |
| `POST` | `/api/v1/ticket/messages` | Kirim pesan teks, balas `202 Accepted` |
| `POST` | `/api/v1/ticket/attachments` | Upload gambar/video (multipart: `file`, `caption`, `client_msg_id`) |
| `POST` | `/api/v1/ticket/resolve` | User menutup tiket sendiri |
| `POST` | `/api/v1/ticket/rate` | Rating `{ "rating": 1-5, "review": "..." }`, hanya jika `resolved`, sekali saja |
| `GET` | `/ws?token=<jwt>` | WebSocket realtime |

**`POST /api/v1/ticket/messages`**

```json
// Request
{ "client_msg_id": "uuid-dari-widget", "message": "Bagaimana input tindakan rawat inap?" }

// Response 202 (balasan AI datang lewat WebSocket, bukan di response ini)
{ "status": true, "message": { /* MessageDTO */ } }
```

**`MessageDTO`** (dipakai REST dan WebSocket, satu bentuk saja):

```json
{
  "id": 10,
  "sender_type": "user",            // user | ai | programmer | system
  "sender_name": "Fajar",
  "message": "teks",
  "attachment": {                   // null jika tidak ada
    "type": "image",                // image | video | document
    "url": "/files/10?exp=1760000000&sig=abc...",
    "mime": "image/png",
    "size": 120394
  },
  "delivery_status": "sent",
  "created_at": "2026-10-09T08:00:05Z"
}
```

### Publik / eksternal

| Method | Path | Catatan |
|---|---|---|
| `GET` | `/files/:message_id?exp=&sig=` | Sajikan lampiran. Signed URL (HMAC atas `message_id` + `exp`), `X-Content-Type-Options: nosniff` |
| `POST` | `/webhook/telegram` | Wajib header `X-Telegram-Bot-Api-Secret-Token` |
| `GET` | `/health` | Liveness |
| `GET` | `/ready` | Readiness (ping database) |

### Event WebSocket

Bentuk: `{"event": "<nama>", "data": { ... }}`

| Event | `data` |
|---|---|
| `helpdesk_new_message` | `MessageDTO` |
| `ai_typing` | `{ "typing": true \| false }` |
| `ticket_claimed` | `{ "programmer_name": "Rayhan", "status": "escalated" }` |
| `ticket_resolved` | `{ "status": "resolved", "message": "..." }` |

> Token ada di query string `/ws`, jadi **jangan** log query string di server maupun reverse proxy.

---

## 9. Roadmap Pengerjaan

Kerjakan **berurutan**. Tiap fase harus bisa dikompilasi dan lolos kriteria *Selesai bila* sebelum lanjut. Satu fase = satu atau beberapa commit langsung di `main`.

### Fase 0 — Setup
**Tujuan:** lahan kerja bersih dan database siap.

- [ ] Tandai backup, tanpa branch baru (kerja langsung di `main`): `git tag backup-sebelum-rewrite c39deeb && git push origin backup-sebelum-rewrite`
- [ ] Hapus file lama, sisakan `internal/database/postgres.go`, `go.mod`, `go.sum`, `scripts/migrate.sh`, `.gitignore`
- [ ] Buat folder: `cmd/server`, `cmd/tenantctl`, `internal/{auth,storage,realtime}`
- [ ] Taruh `config.go` baru dan salin `.env.example` → `.env`, isi nilainya
- [ ] Revoke token bot lama lewat @BotFather, buat token baru
- [ ] Buat database `technova_db`, jalankan `./scripts/migrate.sh up`
- [ ] Siapkan supergroup Telegram: aktifkan **Topics**, jadikan bot admin dengan izin **Manage Topics**, catat `chat_id`

**Selesai bila:** `./scripts/migrate.sh status` menunjukkan versi 1, dan `config.Load()` berhasil dengan `.env` terisi.

> Jangan jalankan `go mod tidy` sebelum ada kode yang memakai dependency. Jalankan di Fase 8.

---

### Fase 1 — Data Layer
**Tujuan:** model dan repository yang cocok persis dengan schema.

- [ ] `tenant/model.go`: `Tenant` (`APIKeyHash`, `AllowedOrigins pq.StringArray`, `IsActive`)
- [ ] `ticket/model.go`: `Ticket`, tipe `Status` dengan konstanta `StatusOpen`, `StatusEscalated`, `StatusResolved`
- [ ] `chat/model.go`: `Message` (`AttachmentPath`, `DeliveryStatus`, `ClientMsgID`, `TelegramMessageID`)
- [ ] Error sentinel `ErrNotFound`, `ErrConflict` (jangan lagi `nil, nil`)
- [ ] Repository, minimal method berikut:

| Repo | Method |
|---|---|
| tenant | `GetByAPIKeyHash`, `GetByID`, `Create` (untuk `tenantctl`) |
| ticket | `Create`, `GetByID`, `GetByPublicID`, `GetActiveByUser`, `GetByThreadID`, `Claim`, `RevertClaim`, `SetThread`, `SetCardMessageID`, `Resolve`, `SaveRating`, `TouchLastUserMessage`, `ListIdle` |
| chat | `Create` (idempoten via `client_msg_id`), `ListByTicket(afterID, limit)`, `GetByID`, `UpdateDelivery`, `ListPendingOutbox` |

- [ ] `Create` tiket menangani pelanggaran unique index (`uq_tickets_active_per_user`) dengan mengambil ulang tiket aktif, bukan error

**Selesai bila:** tes integrasi repository (Postgres sungguhan) lulus, termasuk: dua `Claim` bersamaan → hanya satu yang menang; dua `Create` tiket aktif bersamaan → satu tiket.

---

### Fase 2 — Auth & Tenant CLI
**Tujuan:** semua pintu masuk terautentikasi.

- [ ] `auth.HashAPIKey` (SHA-256 hex), `auth.GenerateAPIKey` (acak 32 byte, prefix `tnk_`)
- [ ] `auth.TokenService`: `Issue(ticket)` dan `Verify(token)` (JWT HS256, klaim `sub=public_id`, `tid`, `uid`, `exp`)
- [ ] Middleware `RequireAPIKey` → cari tenant, tolak jika tidak ada atau `is_active=false`
- [ ] Middleware `RequireTicketToken` → isi context: `ticketPublicID`, `tenantID`, `userID`
- [ ] Middleware `VerifyTelegramSecret` (bandingkan konstan-waktu / `subtle.ConstantTimeCompare`)
- [ ] Middleware CORS `Access-Control-Allow-Origin: *` (tanpa cookie/credentials; keamanan ada di API key + token) dan `CheckOrigin` WS menerima semua origin
- [ ] `cmd/tenantctl`: `add --key kemkes_1101015 --app "KlikMedic SIMRS" --name "RSUD Meuraxa"` (`--origin` opsional, hanya dicatat, tidak dipakai memblokir) → cetak API key **sekali saja**

**Selesai bila:** unit test: API key salah → 401; token kedaluwarsa/dimanipulasi → 401; secret webhook salah → 401. `tenantctl add` menghasilkan tenant yang bisa dipakai memanggil endpoint.

---

### Fase 3 — Tiket
**Tujuan:** siklus hidup tiket tanpa Telegram dan AI.

- [ ] `POST /api/v1/tickets/init` (find-or-create + terbitkan token; `app_name`/`tenant_name` dari tenant)
- [ ] `GET /api/v1/ticket`
- [ ] `POST /api/v1/ticket/resolve` (hanya jika belum `resolved`; set `resolved_at`)
- [ ] `POST /api/v1/ticket/rate` (hanya jika `resolved` dan belum dirating; `409` jika sudah)
- [ ] Kode tiket `TCK-YYYYMMDD-xxxxxx` (6 hex acak)
- [ ] DTO response (model GORM **tidak pernah** dikembalikan langsung)
- [ ] Panggilan `Notifier` pada rating/resolve boleh `nil` dulu (no-op)

**Selesai bila:** dari `curl`: init dua kali untuk user yang sama → tiket yang sama; resolve → init lagi → tiket baru; rate dua kali → yang kedua `409`.

---

### Fase 4 — Realtime
**Tujuan:** hub WebSocket yang aman dan benar.

- [ ] Hub dengan satu goroutine pemilik state (`rooms map[roomKey]map[*Client]struct{}`), channel `register`, `unregister`, `publish`
- [ ] Client: buffer kirim 64, `readPump` + `writePump`, ping/pong
- [ ] **Satu event = satu frame** (jangan gabung dengan `\n`)
- [ ] Client lambat yang bufernya penuh → diputus, bukan memblokir broadcast
- [ ] `GET /ws?token=` → verifikasi token sebelum upgrade, `CheckOrigin` dari cache origin tenant
- [ ] `Publish(roomKey, event, data)` aman dipanggil dari goroutine mana pun

**Selesai bila:** tes dengan dua client di room berbeda: event hanya sampai ke room-nya; client dengan token salah ditolak; tes `-race` bersih.

---

### Fase 5 — AI & Chat
**Tujuan:** user mengobrol dengan AI, semuanya async.

- [ ] `ai.Client`: system prompt, **N pesan terakhir saja** (`AI_HISTORY_LIMIT`), role `user`/`assistant`, timeout dari config, **mengembalikan error** (tidak berpura-pura memberi jawaban palsu)
- [ ] Pesan `programmer` tidak ikut sebagai konteks AI; pesan `user` tidak dikirim dua kali (bug lama)
- [ ] `chat.Service.SendUserMessage`: validasi (maks 4000 karakter, tiket bukan `resolved`) → simpan idempoten → `TouchLastUserMessage` → publish WS → kembalikan `202`
- [ ] Pada pesan pertama: panggil `Notifier.NotifyNewTicket` (kartu sekali), simpan `tg_card_message_id`
- [ ] Jika status `open` dan AI aktif → goroutine: publish `ai_typing` → panggil AI → simpan → publish pesan → `ai_typing=false`
- [ ] Jika AI gagal/dinonaktifkan → pesan `system`: "Tim IT Support akan segera membantu"
- [ ] Satu AI call per tiket pada satu waktu (mutex per tiket)
- [ ] Goroutine memakai context aplikasi + `WaitGroup` supaya ikut graceful shutdown
- [ ] `GET /api/v1/ticket/messages` (paginasi `after_id`)

**Selesai bila:** kirim pesan dengan `client_msg_id` sama dua kali → hanya satu pesan tersimpan; balasan AI tiba lewat WS; AI dimatikan (`AI_API_KEY` kosong) → tetap ada pesan `system`; tidak ada panggilan AI saat status `escalated`.

---

### Fase 6 — Telegram
**Tujuan:** kartu, klaim, topik, dan relay dua arah.

**6a. Client Bot API**
- [ ] Method: `SendMessage`, `CreateForumTopic`, `CloseForumTopic`, `SendPhoto`, `SendVideo`, `AnswerCallbackQuery`, `EditMessageText`, `GetFile`/download
- [ ] Cek status HTTP dan field `ok` di **setiap** panggilan, kembalikan error dengan `description`
- [ ] Tangani `429` (`parameters.retry_after`) dengan tunggu lalu ulang (maks 3x)
- [ ] `html.EscapeString` untuk semua teks dari user/programmer
- [ ] Error tidak boleh memuat URL yang berisi token bot
- [ ] Download file dari Telegram: timeout, batas ukuran, validasi isi (lewat `storage`)

**6b. Webhook**
- [ ] `POST /webhook/telegram` dengan middleware secret → dedup `update_id` (`INSERT ... ON CONFLICT DO NOTHING`) → balas `200` segera → proses di goroutine dengan `recover`
- [ ] Callback `claim:<public_id>`: validasi `chat.id` sama dengan supergroup → `Claim` atomik → `CreateForumTopic` → `SetThread` → sapaan + replay (pesan user + ringkasan jawaban AI terakhir) → edit kartu (hapus tombol) → publish `ticket_claimed` → `AnswerCallbackQuery`
- [ ] Jika `CreateForumTopic` gagal → `RevertClaim` + jawab callback "Gagal membuat topik, coba lagi"
- [ ] Kalah klaim → jawab callback "Sudah diklaim orang lain"
- [ ] Pesan di topik: sesuai tabel routing (bagian 7), termasuk foto/video → download → simpan → publish
- [ ] `/close`, `/selesai` → `Resolve` → publish `ticket_resolved` → `CloseForumTopic`
- [ ] Implementasi `Notifier` (kartu, kirim ke topik, ringkasan rating)

**Selesai bila:** tes dengan server Telegram palsu (`httptest`): klaim dobel → satu topik; `CreateForumTopic` gagal → tiket kembali `open`; pesan di General → tidak ada yang dipublish; update dengan `update_id` sama → diproses sekali; secret salah → 401.

---

### Fase 7 — Storage & Upload
**Tujuan:** lampiran aman.

- [ ] `storage.Save`: `http.MaxBytesReader` (batas per tipe dari config), sniff 512 byte pertama (`http.DetectContentType`), whitelist MIME, nama file acak `<uuid><ext dari MIME>`, folder per tiket (`UPLOAD_DIR/<public_id>/`)
- [ ] `POST /api/v1/ticket/attachments`: simpan → pesan `user` dengan lampiran → publish → kirim ke topik jika `escalated` (atau lampirkan di replay saat klaim jika masih `open`)
- [ ] `GET /files/:message_id?exp&sig`: verifikasi tanda tangan + kedaluwarsa, header `nosniff` dan `Content-Disposition`
- [ ] `MessageDTO.attachment.url` dibuat saat response (TTL 1 jam), bukan disimpan di DB
- [ ] Hapus `static /uploads` publik dari router

**Selesai bila:** upload `.html` yang diganti nama `.png` → ditolak; file melebihi batas → `413`; URL tanpa/ dengan tanda tangan salah/kedaluwarsa → `403`.

---

### Fase 8 — Wiring, Widget, Demo
**Tujuan:** aplikasi utuh bisa dijalankan.

- [ ] `cmd/server/main.go`: config → DB → repo → service → handler → router; `http.Server` dengan timeout; graceful shutdown (`signal.NotifyContext`, hentikan server, tunggu goroutine, tutup DB)
- [ ] `slog` JSON; Gin tanpa `gin.Default()` logger yang mencetak query string `/ws`
- [ ] `web/index.html` (demo) dan `web/assets/widget.js` (alur token baru)
- [ ] Contoh integrasi KlikMedic (CI3): controller `Helpdesk::token()` memanggil `init` dari server dengan API key, mengembalikan token ke halaman; widget memakai token itu
- [ ] `go mod tidy`, `gofmt`, `go vet`

**Selesai bila:** skenario E2E nomor 1–6 (bagian 12) berjalan manual dari halaman demo.

---

### Fase 9 — Hardening & Operasional
**Tujuan:** tahan terhadap gangguan dan penyalahgunaan.

- [ ] Worker outbox (tiap 5 detik): kirim ulang pesan `pending`; `pending` lebih dari 10 menit → `failed`
- [ ] Worker auto-close: `open` idle > 12 jam, `escalated` idle > 72 jam → `resolved` + pesan `system`
- [ ] Worker pembersihan: `telegram_updates` > 7 hari
- [ ] Rate limit (`golang.org/x/time/rate`): `init` 10/menit per IP, pesan 20/menit per tiket, upload 5/menit per tiket
- [ ] Panic recovery middleware, request ID di log
- [ ] `/ready` mengecek database

**Selesai bila:** matikan koneksi Telegram sementara → pesan menumpuk `pending`, lalu terkirim setelah pulih tanpa duplikat; spam pesan → `429`.

---

### Fase 10 — Testing Akhir, Dokumentasi, Deploy
- [ ] Lengkapi unit test service (dengan fake `Notifier`, `Publisher`, AI)
- [ ] Skrip E2E otomatis (`scripts/e2e`) dengan Telegram palsu
- [ ] Tulis ulang `README.md` (tanpa kredensial apa pun)
- [ ] `Dockerfile` (multi-stage) + `docker-compose.yml` (app + postgres)
- [ ] Reverse proxy HTTPS (Caddy/Nginx), `setWebhook` dengan `secret_token`
- [ ] Backup `pg_dump` terjadwal

**Selesai bila:** semua skenario di bagian 12 lulus di lingkungan staging.

---

## 10. Checklist Keamanan

- [ ] Token bot lama sudah di-revoke; `.env` tidak pernah di-commit atau dibagikan
- [ ] Semua endpoint (kecuali `/health`, `/ready`, `/files` bertanda tangan, `/webhook/telegram` bersecret) memerlukan API key atau token tiket
- [ ] API key tenant hanya tersimpan sebagai hash; ditampilkan sekali saat dibuat
- [ ] `TOKEN_SECRET` ≥ 32 karakter, berbeda di tiap environment
- [ ] Webhook: `secret_token` + cek `chat.id` + dedup `update_id`
- [ ] CORS dan WS origin dibatasi ke `allowed_origins` tenant
- [ ] Upload: batas ukuran, MIME sniffing, nama file acak, tidak disajikan sebagai folder statis publik
- [ ] Semua teks ke Telegram di-HTML-escape
- [ ] Response API tidak membocorkan field internal (selalu lewat DTO)
- [ ] Query string `/ws` tidak masuk log (aplikasi dan reverse proxy)
- [ ] Rate limit aktif
- [ ] Data sensitif RS: screenshot bisa memuat data pasien, jadi akses file hanya lewat signed URL berumur pendek

---

## 11. Strategi Testing

| Lapisan | Cara | Fokus |
|---|---|---|
| Repository | Integrasi, Postgres sungguhan (Docker) | Klaim atomik, unique index, idempotensi `client_msg_id` |
| Service | Unit test dengan fake `Notifier`, `Publisher`, AI | Aturan status, aturan routing, kompensasi klaim |
| Telegram client | `httptest` server palsu | Penanganan `ok=false`, `429`, escape HTML |
| Webhook | `httptest` + payload JSON contoh | Secret, dedup, routing General vs topik |
| Realtime | Dua client WebSocket | Isolasi room, `-race` |
| Upload | Tabel kasus (file palsu, terlalu besar, tipe salah) | Whitelist & batas ukuran |
| E2E | Skrip dengan Telegram palsu | Skenario bagian 12 |

Jalankan selalu: `go test -race ./...`

---

## 12. Skenario Uji End-to-End (Acceptance)

1. **Init idempoten:** panggil `init` dua kali untuk user sama → tiket sama, token baru valid.
2. **Pesan pertama:** kirim pesan → muncul di layar, AI menjawab, **satu** kartu muncul di General dengan ringkasan ≤ 150 karakter.
3. **Pesan kedua sebelum klaim:** AI menjawab, **tidak** ada kartu baru.
4. **Klaim:** klik tombol → topik terbentuk dengan judul `[Modul] Nama - Kode`, riwayat user ter-replay, kartu berubah jadi "diklaim", user melihat notifikasi `ticket_claimed`.
5. **Double klaim:** dua programmer menekan tombol bersamaan → satu topik, yang kalah mendapat pesan "sudah diklaim".
6. **Relay dua arah:** balasan programmer di topik muncul di web (badge programmer); balasan user di web muncul di topik; foto dari kedua arah tampil benar; AI **tidak** menyela.
7. **Pesan di General:** programmer mengetik di General → tidak muncul di layar user mana pun.
8. **Selesai & rating:** `/close` → modal rating muncul di web → rating 5 → ringkasan rating muncul di topik → topik tertutup → rating kedua ditolak.
9. **Kegagalan topik:** paksa `createForumTopic` gagal → tiket kembali `open`, tombol klaim masih berfungsi.
10. **Telegram mati sementara:** pesan user saat `escalated` tertahan `pending`, terkirim otomatis setelah pulih, tanpa duplikat.
11. **Keamanan:** token tiket orang lain tidak bisa membaca/mengirim ke tiket ini; webhook tanpa secret → 401; upload file palsu → ditolak; URL file kedaluwarsa → 403.

---

## 13. Deploy & Operasional

**Daftarkan webhook** (ganti placeholder; hindari menyimpan token di riwayat shell, mis. pakai variabel dari file `.env`):

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=https://DOMAIN-ANDA/webhook/telegram" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}" \
  -d 'allowed_updates=["message","callback_query"]'
```

**Perintah harian:**

```bash
./scripts/migrate.sh up            # terapkan migrasi
./scripts/migrate.sh status        # versi saat ini
go run ./cmd/server                # jalankan server
go run ./cmd/tenantctl add ...     # daftar tenant baru
go test -race ./...                # semua tes
```

**Produksi:**
- `APP_ENV=production`, `BASE_URL` wajib `https://`
- HTTPS lewat reverse proxy; teruskan header `Upgrade` untuk WebSocket
- Satu instance cukup. Jika nanti > 1 instance, hub WebSocket in-memory harus diganti pub/sub (Redis)
- Backup `pg_dump` harian; folder `UPLOAD_DIR` ikut dibackup
- Pantau: `/ready`, jumlah pesan `pending`/`failed`, error rate Telegram

---

## 14. Konvensi Kerja

- **Branch:** kerja langsung di `main`. Kode lama tetap aman di tag `backup-sebelum-rewrite` (lihat isinya dengan `git show backup-sebelum-rewrite:<path>`, ambil file dengan `git checkout backup-sebelum-rewrite -- <path>`).
- **Commit:** satu fase = beberapa commit kecil. Format: `feat(ticket): atomic claim`, `fix(realtime): one frame per event`, `test(chat): idempotent client_msg_id`.
- **Error:** dibungkus dengan `fmt.Errorf("...: %w", err)`; jangan membuang error dengan `_ =` kecuali ada komentar alasannya.
- **Context:** setiap fungsi yang menyentuh DB/HTTP menerima `context.Context`.
- **Log:** `slog` dengan field terstruktur (`ticket`, `tenant`, `update_id`); tidak ada rahasia di log.
- **DTO:** handler hanya menerima/mengembalikan DTO, tidak pernah model GORM.
- **Migrasi:** setelah ada data nyata, jangan edit migrasi lama; buat `000002`, dst.

---

## 15. Risiko & Catatan

| Risiko | Mitigasi |
|---|---|
| Rate limit Telegram (429) saat banyak tiket | Retry dengan `retry_after`, outbox, kirim replay bertahap |
| Biaya/latensi AI | Batasi riwayat (`AI_HISTORY_LIMIT`), timeout, rate limit pesan per tiket |
| Screenshot memuat data pasien | Signed URL pendek, tidak ada folder publik, pertimbangkan kebijakan retensi file |
| Hub in-memory tidak skalabel | Cukup untuk 1 instance; migrasi ke Redis pub/sub bila perlu |
| Topik Telegram menumpuk | `closeForumTopic` saat resolved; pertimbangkan hapus topik lama |
| Widget lama (README lama) tidak kompatibel | Alur token baru dijelaskan di Fase 8; widget ditulis ulang |

**Keputusan yang masih terbuka (tidak menghalangi fase awal):**
- Kebijakan retensi file lampiran (berapa lama disimpan).
- Apakah dashboard admin sederhana (daftar tiket, rating) dibutuhkan setelah v1.