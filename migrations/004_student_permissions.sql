-- ================================================================
-- MIGRASI 004: Hak akses students (Tugas Mandiri C.1)
-- Jalankan SETELAH 003_rbac.sql.
-- ================================================================

INSERT INTO permissions (name, description) VALUES
    ('student:list',       'Melihat daftar seluruh mahasiswa'),
    ('student:read:any',   'Melihat data mahasiswa mana pun'),
    ('student:create',     'Menambahkan data mahasiswa'),
    ('student:update:any', 'Mengubah data mahasiswa mana pun'),
    ('student:delete',     'Menghapus data mahasiswa')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name) VALUES
    ('admin', 'student:list'),
    ('admin', 'student:read:any'),
    ('admin', 'student:create'),
    ('admin', 'student:update:any'),
    ('admin', 'student:delete'),
    ('staff', 'student:list'),
    ('staff', 'student:read:any'),
    ('staff', 'student:create')
ON CONFLICT DO NOTHING;

-- ----------------------------------------------------------------
-- owner_id, siapa yang mendaftarkan data mahasiswa ini.
--
-- NULLABLE, dan itu disengaja: 6 baris data lama dibuat sebelum ada
-- login, jadi memang TIDAK punya pemilik. Mengisinya asal-asalan
-- (misalnya ke user id 1) sama saja memberi hak milik palsu.
-- NULL = "tak bertuan" -> hanya role dengan permission :any yang boleh
-- menyentuhnya (fail closed).
--
-- Karena kolom baru langsung NULL untuk semua baris lama, FOREIGN KEY
-- bisa dipasang tanpa gagal di tengah jalan. NOT NULL sengaja TIDAK
-- dipasang: kalau dipaksa, migration pasti gagal di baris lama.
--
-- ON DELETE SET NULL: user dihapus -> data mahasiswanya tetap ada,
-- hanya kembali jadi tak bertuan.
-- ----------------------------------------------------------------
ALTER TABLE students
    ADD COLUMN IF NOT EXISTS owner_id INTEGER
    REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS students_owner_id_idx ON students (owner_id);
