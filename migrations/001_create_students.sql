-- ============================================================
-- Tabel students
-- Praktikum Pemrograman Backend Lanjut - Pertemuan 3
-- ============================================================

CREATE TABLE IF NOT EXISTS students (
    id         SERIAL       PRIMARY KEY,
    nim        VARCHAR(12)  NOT NULL,
    name       VARCHAR(100) NOT NULL,
    grade      NUMERIC(5,2) NOT NULL DEFAULT 0,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Batasan isi dijaga basis data, bukan hanya oleh validasi Go.
    -- Kode Go bisa dilewati (lewat psql langsung, skrip lain, atau bug),
    -- sedangkan batasan di sini berlaku untuk siapa pun yang menulis.
    CONSTRAINT students_grade_range CHECK (grade >= 0 AND grade <= 100),
    CONSTRAINT students_nim_digits  CHECK (nim ~ '^[0-9]{9,12}$'),
    CONSTRAINT students_name_length CHECK (LENGTH(TRIM(name)) >= 3)
);

-- ============================================================
-- Indeks
-- ============================================================

-- 1) Keunikan NIM.
--    Inilah yang menggantikan pemeriksaan manual nimDipakai() pada
--    pertemuan 2. Dibuat sebagai UNIQUE INDEX, bukan UNIQUE constraint
--    biasa, agar bisa memakai ekspresi bila kelak diperlukan.
CREATE UNIQUE INDEX IF NOT EXISTS students_nim_key
    ON students (nim);

-- 2) Indeks untuk pencarian nama.
--    Endpoint daftar memakai ILIKE pada kolom name. Tanpa indeks,
--    setiap pencarian memaksa PostgreSQL membaca seluruh tabel.
--    Indeks pada LOWER(name) mempercepat pencocokan yang tidak
--    membedakan huruf besar dan kecil.
CREATE INDEX IF NOT EXISTS students_name_lower_idx
    ON students (LOWER(name));

-- 3) Indeks untuk penyaringan status aktif yang digabung pengurutan id.
--    Query daftar hampir selalu berbentuk
--    WHERE is_active = $1 ORDER BY id, sehingga indeks gabungan
--    melayani penyaringan dan pengurutan sekaligus.
CREATE INDEX IF NOT EXISTS students_is_active_id_idx
    ON students (is_active, id);

-- ============================================================
-- Data awal
-- ============================================================

INSERT INTO students (nim, name, grade, is_active) VALUES
    ('230001001', 'Rafi Fernandito', 88.50, TRUE),
    ('230001002', 'Andi Pratama',    85.00, TRUE),
    ('230001003', 'Citra Dewi',      92.25, FALSE),
    ('230001004', 'Budi Santoso',    78.00, TRUE),
    ('230001005', 'Eka Lestari',     64.75, FALSE),
    ('230001006', 'Fajar Nugroho',   71.50, TRUE)
ON CONFLICT (nim) DO NOTHING;
