# api-students-queue (Praktikum Backend Lanjut Pertemuan 11)

Background job dan pemrosesan asinkron dengan Asynq + Redis. Lanjutan `api-students-mongo`
(pertemuan 10). Email konfirmasi KRS dikirim worker di background lewat transactional outbox.

**Rafi Fernandito Setiawan, 434241117, D4 Teknik Informatika, Fakultas Vokasi, Universitas Airlangga**

## Menjalankan

Butuh Redis di `localhost:6379`. Di Windows dipakai Memurai Developer
(`winget install -e --id Memurai.MemuraiDeveloper`, cek dengan `memurai-cli ping`).

```powershell
Copy-Item ..\api-students-mongo\.env .env
Add-Content .env "`nREDIS_ADDR=localhost:6379`nREDIS_PASSWORD=`nREDIS_DB=0`nWORKER_CONCURRENCY=5`nRELAY_INTERVAL_MS=1000`nRELAY_BATCH=100`nMAILER_DELAY_MS=1500`nMAILER_FAIL_EVERY=0"
$env:PGPASSWORD = "praktikum123"
psql -U postgres -d praktikum_backend -f migrations/010_outbox.sql
psql -U postgres -d praktikum_backend -f migrations/011_notification_recipient_key.sql
go get github.com/hibiken/asynq@v0.26.0
go mod tidy
go build ./... ; go vet ./... ; go test ./...

go run . -role=api       # terminal 1
go run . -role=worker    # terminal 2
```

`008_outbox.sql` di modul menjadi `010_outbox.sql`, dan "migration 009" pada Tugas Mandiri D.2 menjadi
`011_notification_recipient_key.sql`, karena nomor 008 dan 009 sudah terpakai.

Log API ditulis ke `logs/api.log`, log worker ke `logs/worker.log`, email tiruan ke `logs/mail.log`.

## Pengujian (Spesifikasi C dan Tugas Mandiri D.2)

```powershell
go run ./tools/uji11 -mode siapkan   # daftar dan login akun mhsuji dan salahalamat
go run ./tools/uji11 -mode c1        # jalur normal
go run ./tools/uji11 -mode c2        # matikan Redis dulu, alat menunggu Redis dinyalakan lagi
go run ./tools/uji11 -mode c3        # worker dengan MAILER_FAIL_EVERY=3
go run ./tools/uji11 -mode c4        # worker dengan MAILER_FAIL_EVERY=0
go run ./tools/uji11 -mode c5        # mulai dengan worker mati, alat menunggu worker dinyalakan
go run ./tools/uji11 -mode c6        # worker dengan MAILER_DELAY_MS=5000, tekan Ctrl+C di worker
go run ./tools/uji11 -mode kuota     # D.2, worker dengan MAILER_FAIL_EVERY=3
go run ./tools/redeliver -outbox <id> -copies 5
```

## Endpoint baru

| Method | Path | Hak akses |
|---|---|---|
| GET | `/api/v1/notifications/mine` | login |
| GET | `/api/v1/admin/queue` | `queue:monitor` |

`PATCH /api/v1/courses/:id` kini mencatat event `course.quota_changed` ke outbox, dan setiap peserta
kelas menerima email perubahan kuota.
