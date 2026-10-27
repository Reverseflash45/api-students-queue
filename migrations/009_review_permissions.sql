-- Migration 009: permission moderasi ulasan (di modul: 007_review_permissions.sql)
--   psql -U postgres -d praktikum_backend -f migrations/009_review_permissions.sql
--
-- Ulasannya disimpan di MongoDB, tetapi hak aksesnya tetap dikelola di
-- PostgreSQL bersama permission lain. Satu sistem hak akses untuk seluruh
-- aplikasi, apa pun database tempat datanya disimpan.
INSERT INTO permissions (name, description) VALUES
    ('review:moderate', 'Menghapus ulasan milik siapa pun')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name) VALUES
    ('admin', 'review:moderate')
ON CONFLICT DO NOTHING;
