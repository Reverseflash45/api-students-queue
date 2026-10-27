-- Migration 011: idempotency key per penerima (Tugas Mandiri D.2)
-- (di modul: "migration 009")
--   psql -U postgres -d praktikum_backend -f migrations/011_notification_recipient_key.sql
--
-- Event course.quota_changed menghasilkan BANYAK notifikasi, satu untuk
-- setiap peserta kelas. UNIQUE (outbox_id) akan menolak notifikasi kedua
-- dari event yang sama, jadi kuncinya diganti menjadi pasangan
-- (outbox_id, user_id): satu event, satu notifikasi PER PENERIMA.
--
-- Untuk event KRS biasa tidak ada yang berubah. Event itu hanya punya satu
-- penerima, jadi (outbox_id, user_id) sama ketatnya dengan outbox_id saja.
--
-- user_id dipilih, bukan alamat email, karena email bisa diganti pemiliknya
-- di antara dua retry; user_id tidak pernah berubah.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_outbox_unique;

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_outbox_user_unique;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_outbox_user_unique UNIQUE (outbox_id, user_id);
