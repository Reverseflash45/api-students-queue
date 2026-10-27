-- ================================================================
-- MIGRASI 005: index untuk keyset pagination (Pertemuan 7)
--
-- Urutan column pada index HARUS sama persis dengan ORDER BY pada
-- query, termasuk arah DESC-nya. Bila tidak, PostgreSQL tetap dapat
-- memakai index untuk sebagian kasus, tetapi tidak dapat memakainya
-- untuk melompat langsung ke posisi cursor, dan seluruh keuntungan
-- keyset pagination hilang tanpa satu pun pesan kesalahan.
-- ================================================================

CREATE INDEX IF NOT EXISTS students_created_at_id_desc_idx
    ON students (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS users_created_at_id_desc_idx
    ON users (created_at DESC, id DESC);
