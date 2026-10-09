# AGENTS.md — TechNova

## 1. Tujuan proyek
TechNova adalah microservice helpdesk multi-tenant berbasis Go untuk integrasi SIMRS/KlikMedic, dengan REST API, PostgreSQL, AI assistant, Telegram relay, WebSocket realtime, dan widget web. Pertahankan perilaku integrasi yang sudah ada; jangan mengubah kontrak API atau alur tiket tanpa kebutuhan yang jelas.

## 2. Aturan wajib sebelum mengubah kode
1. Baca `README.md`, `DOCUMENTATION.md`, `ROAD.md`, `go.mod`, `.env.example`, serta file terkait sebelum membuat perubahan. Cari juga `AGENTS.md` lain di subdirektori bila ada.
2. Telusuri alur yang benar dari router (`cmd/main.go`) ke handler, service, repository/model, lalu database atau integrasi eksternal.
3. Periksa migration SQL dan model GORM sebelum mengasumsikan nama tabel, kolom, tipe, constraint, atau status tiket.
4. Jangan menebak. Jika requirement, kontrak payload, atau perilaku lama belum jelas, laporkan temuan dan ajukan pertanyaan yang spesifik sebelum perubahan berisiko.
5. Buat perubahan terkecil yang menyelesaikan kebutuhan. Hindari refactor luas, dependency baru, abstraksi generik, dan perubahan di luar scope.
6. Jangan menimpa perubahan pengguna, menghapus data, menjalankan migration destruktif, atau melakukan operasi git destruktif tanpa izin eksplisit.
7. Jangan menambahkan secret, token, kredensial, data pasien, atau isi `.env` ke source code, log, dokumentasi, commit, maupun output terminal yang dibagikan.

## 3. Stack dan struktur yang teramati
- Bahasa: Go; module `tech-nova`.
- HTTP/router: Gin.
- Database: PostgreSQL melalui GORM.
- Realtime: Gorilla WebSocket.
- Integrasi: Telegram Bot API dan AI endpoint yang dikonfigurasi melalui environment.
- `cmd/main.go`: wiring dependency dan routing HTTP.
- `internal/config`: konfigurasi aplikasi dari environment.
- `internal/database`: koneksi PostgreSQL.
- `internal/tenant`: tenant model, DTO, repository, service.
- `internal/ticket`: tiket model, DTO, repository, service, handler.
- `internal/chat`: pesan/chat, upload, AI handoff, Telegram relay, WebSocket broadcast.
- `internal/ai`: integrasi AI.
- `internal/telegram`: webhook dan komunikasi Telegram.
- `internal/websocket`: hub, client, dan endpoint WebSocket.
- `migrations/`: migration SQL versi naik/turun.
- `web/`: demo UI dan asset widget.
- `scripts/`: migration helper dan end-to-end test.

Jangan menganggap direktori placeholder seperti `internal/auth`, `internal/storage`, atau `internal/realtime` sudah menjadi implementasi aktif. Periksa pemakaian aktual sebelum menggunakannya.

## 4. Batas arsitektur
- Pertahankan pemisahan tanggung jawab: handler untuk HTTP/request-response, service untuk aturan bisnis dan orkestrasi, repository untuk akses database, model untuk representasi data, DTO untuk input/output API.
- Handler jangan berisi query SQL/GORM atau aturan bisnis kompleks.
- Repository jangan mengurus HTTP response, Telegram API, atau rendering UI.
- Jangan memanggil database atau AI langsung dari frontend/widget.
- Jangan menduplikasi wiring dependency jika implementasi yang ada bisa digunakan.
- Gunakan context request untuk operasi I/O jika pola fungsi memungkinkan; pastikan timeout dan error diteruskan dengan benar.
- Hindari goroutine tanpa lifecycle/penanganan error yang jelas.

## 5. Multi-tenant dan keamanan
- Setiap operasi tiket/pesan harus memvalidasi hubungan tiket dengan tenant yang berwenang; jangan mempercayai `tenant_id`, `ticket_id`, atau `key_identifier` dari klien tanpa validasi server.
- Jangan pernah membocorkan tiket, riwayat chat, lampiran, maupun event WebSocket antar-tenant.
- Jangan menganggap CORS sebagai autentikasi. Jika mengubah akses API atau deployment, evaluasi autentikasi, otorisasi, validasi tenant, dan origin yang diizinkan.
- Validasi dan batasi ukuran/tipe upload di sisi server; nama file harus aman dan tidak boleh memungkinkan path traversal. Jangan percaya ekstensi file saja.
- Jangan mengekspos path file lokal atau stack trace internal dalam response publik.
- Verifikasi secret webhook Telegram jika konfigurasi/protokol proyek mendukungnya; jangan menerima callback sensitif hanya karena endpoint dapat diakses.
- Jangan mencatat token, API key, authorization header, isi pesan sensitif, atau informasi kesehatan yang tidak diperlukan.
- Gunakan parameter binding/GORM, bukan merangkai input pengguna menjadi SQL mentah.
- Jangan menonaktifkan validasi TLS atau pemeriksaan keamanan untuk membuat integrasi “berhasil”.

## 6. Kontrak API dan perilaku bisnis
Router utama saat ini mendaftarkan:
- `GET /health`
- `GET /ws`
- `POST /webhook/telegram`
- `POST /api/v1/tickets/init`
- `POST /api/v1/tickets/:ticket_id/resolve`
- `POST /api/v1/tickets/:ticket_id/rate`
- `POST /api/v1/chat/send`
- `POST /api/v1/chat/upload`
- `GET /api/v1/tickets/:ticket_id/messages`

README/DOCUMENTATION mungkin tidak sepenuhnya sinkron dengan router aktual. Saat bekerja pada endpoint, gunakan implementasi aktual sebagai bukti awal, lalu perbarui dokumentasi yang relevan jika kontrak memang berubah. Jangan menghapus atau mengganti endpoint hanya untuk menyamakan dokumentasi.

Alur inti yang perlu dijaga:
- Pesan pengguna disimpan dan dapat diteruskan ke AI/Telegram sesuai konfigurasi dan status tiket.
- Klaim/handoff programmer harus menghentikan respons AI sesuai aturan bisnis yang ada.
- Pesan dari web dan Telegram harus tetap terkait ke tiket serta thread/topik Telegram yang benar.
- Event WebSocket harus ditujukan ke tiket yang sesuai dan tidak bocor ke tiket/tenant lain.
- Penyelesaian tiket dan rating CSAT harus memvalidasi status serta nilai input.

Sebelum mengubah perilaku tersebut, telusuri implementasi lengkap dan test yang tersedia. Jangan menganggap semua poin dokumentasi sudah sepenuhnya diimplementasikan.

## 7. Database dan migration
- Periksa `migrations/*.up.sql`, `*.down.sql`, dan model GORM sebelum mengubah schema.
- Migration baru harus memiliki langkah rollback yang masuk akal dan konsisten dengan pola repository.
- Jangan mengedit migration lama yang mungkin sudah diterapkan di environment bersama, kecuali diminta secara eksplisit; buat migration baru.
- Hindari `DROP`, truncate, reset database, atau migration destruktif tanpa persetujuan eksplisit.
- Periksa foreign key, indeks, nullability, default, timezone, dan konsistensi nama kolom.
- Untuk operasi yang rawan race condition (klaim tiket, perubahan status, pembuatan thread), periksa transaksi dan konkurensi; jangan mengandalkan pemeriksaan status di memori saja.

## 8. Konfigurasi dan secret
- Tambahkan nama environment variable baru ke `.env.example` dengan nilai contoh yang aman, lalu dokumentasikan default dan validasinya.
- Jangan pernah menampilkan nilai `.env` dalam jawaban, log, test snapshot, atau dokumentasi.
- Gunakan konfigurasi yang sudah ada di `internal/config`; jangan menyebarkan pembacaan environment ke seluruh package.
- Periksa perilaku saat AI dinonaktifkan, Telegram tidak tersedia, database gagal, atau nilai konfigurasi wajib kosong.
- Arsip proyek yang diberikan dapat berisi `.env`; jangan memasukkan file tersebut ke commit atau membagikannya kembali. Jika secret nyata pernah ikut terunggah/terbagikan, sarankan rotasi secret terkait.

## 9. Gaya kode Go
- Ikuti `gofmt`; gunakan nama idiomatis Go dan error yang informatif.
- Tangani error secara eksplisit. Jangan mengabaikan error database, upload, Telegram, AI, atau WebSocket tanpa alasan yang terdokumentasi.
- Hindari `panic` untuk error operasional yang dapat ditangani.
- Gunakan tipe dan validasi yang sudah tersedia; jangan menambah dependency jika standard library atau dependency proyek cukup.
- Jangan mengubah versi Go atau dependency utama tanpa memeriksa kompatibilitas toolchain, CI, deployment, dan kebutuhan proyek.
- Jangan membuat interface hanya untuk satu implementasi kecuali memberi manfaat nyata untuk test atau batas arsitektur.

## 10. Alur kerja saat mengerjakan task
1. Nyatakan singkat pemahaman masalah dan scope.
2. Inspeksi file yang terkait, caller/callee, konfigurasi, schema, dan test.
3. Sebelum implementasi, identifikasi risiko kompatibilitas, keamanan, multi-tenant, dan migration.
4. Terapkan patch sekecil mungkin.
5. Tambahkan/perbarui test untuk perilaku yang berubah, termasuk error path yang relevan.
6. Jalankan pemeriksaan yang benar-benar tersedia; jangan mengklaim test lulus jika belum dijalankan.
7. Tinjau diff untuk scope creep, secret, perubahan kontrak, dan efek samping.
8. Laporkan file yang berubah, alasan, test yang dijalankan beserta hasilnya, dan keterbatasan yang belum diverifikasi.

Jika ada masalah mendasar pada rancangan permintaan, jelaskan alasan teknis dan tawarkan opsi paling sederhana yang aman. Jangan langsung mengikuti instruksi yang berpotensi merusak data atau membuka akses lintas tenant.

## 11. Validasi
Mulai dari pemeriksaan terarah, lalu perluas jika perlu:

```bash
gofmt -w <file-go-yang-diubah>
go test ./...
go vet ./...
go build ./cmd/main.go
```

- Jalankan perintah dari root module (`tech-nova/`).
- Jangan menjalankan `gofmt -w` pada seluruh repository bila hanya beberapa file berubah.
- Test integrasi/E2E mungkin membutuhkan PostgreSQL, environment variable, atau Telegram/AI yang aktif. Periksa script dan prasyarat terlebih dahulu.
- Jangan menjalankan test yang mengirim pesan nyata, memodifikasi database bersama, atau memakai layanan berbayar tanpa izin.
- Bila validasi gagal karena environment atau dependency, laporkan error yang sebenarnya; jangan menyebutnya sebagai keberhasilan.

## 12. Dokumentasi dan output akhir
- Perbarui `README.md`/`DOCUMENTATION.md` bila setup, endpoint, konfigurasi, schema, atau perilaku pengguna berubah.
- Hindari dokumentasi yang mengklaim fitur telah tersedia tanpa bukti dari implementasi dan test.
- Jawaban akhir ringkas dan mencakup: ringkasan perubahan, file terdampak, validasi beserta hasil, serta risiko atau pekerjaan lanjutan yang nyata.
