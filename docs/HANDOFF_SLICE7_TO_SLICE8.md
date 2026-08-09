# OPENCLOUD — SLICE 7 HANDOFF + NEXT TASK PROMPT

> Berikan file ini ke AI berikutnya sebagai konteks utama. AI tersebut harus
> membaca `CLAUDE.md`, `docs/OPENCLOUD_MASTER_IMPLEMENTATION_PROMPT.md`, dan
> dokumen di bawah ini SEBELUM menulis kode apa pun.

---

## 0. REPOSITORY & BRANCH STATE (per 10 Aug 2026)

- **Repo:** github.com/Nafixhutao/opencloud
- **Main branch:** `main` — sudah berisi SLICE 1–6 + SLICE 7 dasar (merge PR #33, commit `21c9fa9`)
- **Branch aktif SLICE 7 hardening:** `feature/slice-7-database-manager-hardening` — commit `3d26599`, **PR #34 masih OPEN, CI PASS, belum di-merge**
- **Yang harus dilakukan AI baru PERTAMA KALI:**
  1. `git fetch origin && git checkout main && git pull origin main`
  2. Review & merge PR #34 (atau cherry-pick `2540f89` + `3d26599` ke main)
  3. Baru mulai SLICE 8

---

## 1. STATUS SLICE (dari master prompt)

| Slice | Status | Keterangan |
|---|---|---|
| 1–5 | ✅ Complete | Project model, build, builder, registry, logs |
| 6 | ✅ Complete (di main) | Env/Secrets (AES-256-GCM) |
| 7 | 🚧 90% (PR #34 pending) | Database Manager + SQL Console |
| **8** | ⬜ **BELUM DIMULAI** | **Object Storage — tugas berikutnya** |

---

## 2. APA YANG SUDAH DIBUAT DI SLICE 7 (jangan diulang)

### Backend (semua ada di `backend/internal/`)
| Komponen | File | Catatan |
|---|---|---|
| Model | `model/database_console_session.go`, `model/console_query_audit.go` | Session punya `actor_id, engine, status(active/revoked/expired), revoked_at`; audit punya `statement_type` |
| Repository | `repository/database_console_session.go`, `repository/console_query_audit.go` | Session: soft-revoke, mark-expired, cleanup 7 hari; Audit: list by account/session |
| Repository | `repository/managed_database.go` | **`GetCredential()` NON-destructive** (untuk console, jangan diubah) |
| Service | `service/database_console_session.go` | Create: validasi ownership + status active; Validate; Revoke; Cleanup |
| Service | `service/console_query.go` | **Real SQL execution** via read-only transaction (`SET TRANSACTION READ ONLY` / `SET SESSION TRANSACTION READ ONLY`), `statement_timeout`, MAX_EXECUTION_TIME, row cap 1000, query cap 10K, multi-statement diblok, redacted errors |
| Service | `service/managed_database.go` | `ConsoleCredentials()` decrypt non-destructive |
| Handler | `handler/database_console_session.go`, `handler/console_query.go` | Gunakan `middleware.AccountID/UserID`, `respondError`, `apperr` |
| Server | `server/server.go` | Wiring: session + query service butuh `databaseRepo`, `databaseCipher`, `cfg.CustomerDatabases.Enabled` |
| Migrations | `20260809040000` (sessions), `20260809050000` (audit), `20260809060000` (harden) | Semua sudah checksummed di `checksums.sha256` |

### Frontend (repo root)
- `lib/database-console-sessions.ts` — client API (via BFF `/api/databases/:id/console/...`)
- `app/(dashboard)/databases/[id]/page.tsx` — server component, fetch via `apiJSON`
- `components/databases/database-console.tsx` — UI: session start/revoke, Run/Explain/Format/Cancel/Clear
- BFF proxy: `app/api/databases/[id]/console/{sessions,execute}` (pattern `proxyAPI` dari `lib/api-route.ts`)

### Konvensi penting (sudah dipakai, wajib dilanjutkan)
- Error: `apperr.*` (bukan error string), respond via `respondError`
- Tenant scope: selalu `account_id` di query repo
- Credentials: cipher AES-256-GCM, decrypt hanya di memory, `defer clear(plaintext)`
- Tests: `internal/service/console_query_test.go` (detectStatementType, multi-statement, TTL)

---

## 3. SISA YANG BELUM DILAKUKAN DI SLICE 7 (deferred, bukan prioritas)

Hanya lakukan jika diminta eksplisit:
- `db.<platform-domain>` gateway + phpMyAdmin untuk MariaDB (master prompt §19)
- Temporary DB user per console session (master prompt §21) — saat ini pakai credential customer yang sudah ada
- UI schema/table browser ala Supabase Studio (master prompt §20) — saat ini cuma SQL console
- Import/export SQL dump async (master prompt §24)
- Database insights/backups UI (master prompt §25-26)

---

## 3.5. KEPUTUSAN PEMILIK PRODUK (10 Aug 2026)

**Object Storage SLICE 8 → SELF-HOSTED MinIO (S3-compatible), BUKAN AWS cloud.**

- Production provider: **MinIO** di server sendiri, diakses via **aws-sdk-go-v2**
  (endpoint diarahkan ke MinIO; kode identik jika nanti migrasi ke AWS S3/Cloudflare R2)
- Dependency `github.com/aws/aws-sdk-go-v2` BELUM ada di `go.mod` → tambahkan
  sebagai "Adopted — Phase 4L" + update `docs/DEPENDENCIES.md`, `backend/go.mod`,
  `backend/go.sum` pada commit yang sama dengan consumer pertama (aturan
  DEPENDENCIES.md §1)
- `FakeStorageProvider` TETAP wajib untuk CI (master prompt §60: CI tidak boleh
  bergantung pada production object storage)
- Config MinIO: env vars `S3_ENDPOINT`/`S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY`/
  `S3_REGION`/`S3_BUCKET_PREFIX`; jangan hardcode, jangan log
- MinIO runtime: Docker Compose service (cek `docker-compose.yml`/`deploy/`)
  + dokumentasi di `docs/INFRASTRUCTURE.md`
- **Keputusan host:** MinIO DI-INCLUDE ke `docker-compose.yml` sebagai service
  (satu server, rootless, volume terpisah, port internal saja — tidak expose ke internet)
- **Keputusan final (10 Aug):** provider = **MinIO** (bukan SeaweedFS/Garage/Ceph/RustFS)
- **Keputusan final bucket layout (10 Aug):** **1 MinIO bucket per tenant**
  (`opencloud-<account_id>`), di dalamnya **logical buckets per fitur** di
  kontrol-plane (model `storage_buckets` = `avatars`, `documents`, dll) — pola
  Supabase Storage. Isolasi tenant = bucket level MinIO + account_id scope di
  repository. Delete account = delete 1 MinIO bucket.

---

## 4. 🎯 TUGAS BERIKUTNYA: SLICE 8 — OBJECT STORAGE

> Berikut adalah prompt yang bisa diberikan langsung ke AI:

---

**SLICE 8 — OBJECT STORAGE (S3-compatible, Supabase-Storage-like)**

Baca dulu: `CLAUDE.md`, `docs/OPENCLOUD_MASTER_IMPLEMENTATION_PROMPT.md` §32-37,
`docs/FRONTEND.md`, `docs/BACKEND.md`, `docs/DATABASE.md`, `docs/SECURITY.md`,
`docs/API.md`. Pelajari pola kode yang SUDAH ADA di `backend/internal/` —
SLICE 7 (database console) dan SLICE 6 (env vars) adalah template terbaik:
model → repository → service → handler → server wiring → migration →
BFF route → UI component → tests.

**Scope (vertical slice kecil, sesuai §64 master prompt):**

1. **Schema (migration baru, JANGAN edit migration lama):**
   - `storage_buckets`: id, account_id, project_id (nullable), name, visibility (public/private), quota_bytes, size_bytes, status, created_at, updated_at, deleted_at
   - `storage_objects`: id, account_id, bucket_id, key (virtual folder path), size_bytes, content_type, etag, metadata (jsonb), created_at, deleted_at
   - Semua customer rows wajib `account_id`
   - Update `checksums.sha256` setelah migration (format: `hash  nama_file` TANPA prefix path)

2. **Provider abstraction (`internal/storage/`):** `ObjectStorageProvider` interface — `CreateBucket, DeleteBucket, PutObject, GetObject, DeleteObject, PresignUpload, PresignDownload`; implement `FakeStorageProvider` untuk CI; `S3StorageProvider` dengan S3-compatible backend (AWS SDK / MinIO client — **cek dulu apakah dependency sudah diizinkan** di `docs/DEPENDENCIES.md`). Jangan lock ke satu provider.

3. **Backend layering (WAJIB):** model → repository (scoped by account_id) → service (business logic, validasi quota/MIME/size/path traversal, tenant isolation) → handler (Gin, `middleware.AccountID`, `apperr`) → register di `server/server.go`:
   - `GET/POST /api/v1/storage/buckets`
   - `GET/DELETE /api/v1/storage/buckets/:bucketId`
   - `GET/POST /api/v1/storage/buckets/:bucketId/objects` (list/upload metadata)
   - `GET/DELETE /api/v1/storage/objects/:objectId`
   - `POST /api/v1/storage/objects/:objectId/presign` (upload/download signed URL)

4. **Keamanan (master prompt §37):** content length, MIME check, filename/key policy, path traversal (`..`), quota per bucket, tenant ownership. **JANGAN** simpan binary di PostgreSQL — object data ke S3 backend, kontrol-plane simpan metadata saja.

5. **Frontend (pattern SLICE 7):**
   - `lib/storage.ts` + BFF route `app/api/storage/...` (pakai `proxyAPI`)
   - Halaman `app/(dashboard)/storage/` + `components/storage/storage-browser.tsx` — file browser (upload, folder, delete, copy URL, signed URL), state loading/empty/error
   - Sidebar: tambah link Storage (cek `app/(dashboard)/layout.tsx`)

6. **Tests:** unit test tenant isolation, path traversal, quota; fake provider untuk CI. Frontend test untuk state utama (pakai Vitest + Testing Library, contoh `components/databases/database-dashboard.test.tsx`).

7. **Docs:** update `ROADMAP.md` (tandai SLICE 8), `docs/API.md`, `CHANGELOG.md`.

**PENTING:**
- Ikuti `CLAUDE.md` pasal 5 & 6 — layering, tenant scoping, no new dependency tanpa approval, jangan edit migration terkirim
- Ikuti pola kode yang ada — jangan bikin abstraksi baru yang tidak perlu
- **STOP sebelum commit/push** — tunjukkan perubahan, jalankan `gofmt`, `go vet`, `go build`, `go test ./...`, lint & tsc; laporkan hasil jujur
- Kalau ada keputusan arsitektur yang ambigu → tanya dulu, jangan asumsi

---

## 5. COMMAND REFERENCE

```bash
# Backend (backend/)
export PATH="/tmp/go/bin:$PATH"   # Go 1.26.5 sudah di-download di /tmp/go
go build ./... && go vet ./... && go test ./...
gofmt -l .                        # harus kosong
# Migration checksums (dari backend/migrations/)
sha256sum *.sql | sed 's|  migrations/|  |' > checksums.sha256

# Frontend (repo root)
npx tsc --noEmit
npm run lint
```
