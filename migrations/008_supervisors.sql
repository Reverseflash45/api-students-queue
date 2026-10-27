-- Migration 008: dosen pembimbing tugas akhir (Tugas Mandiri D.2)
--   psql -U postgres -d praktikum_backend -f migrations/008_supervisors.sql

CREATE TABLE IF NOT EXISTS supervisors (
    id               SERIAL       PRIMARY KEY,
    name             VARCHAR(120) NOT NULL UNIQUE,
    quota            INTEGER      NOT NULL CHECK (quota > 0),
    supervised_count INTEGER      NOT NULL DEFAULT 0,
    version          INTEGER      NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Sama seperti courses: bila kode benar, constraint ini tidak pernah
    -- terpicu. Bila terpicu, ada bug, dan dijawab 500.
    CONSTRAINT supervisors_count_within_quota
        CHECK (supervised_count >= 0 AND supervised_count <= quota)
);

CREATE TABLE IF NOT EXISTS supervisions (
    -- student_id dijadikan PRIMARY KEY. Itulah yang menjamin di level
    -- database bahwa satu mahasiswa hanya punya SATU pembimbing: baris
    -- kedua untuk mahasiswa yang sama selalu ditolak, seserentak apa pun
    -- datangnya.
    --
    -- ON DELETE RESTRICT: mahasiswa yang masih punya pembimbing tidak
    -- bisa dihapus diam-diam, karena supervised_count akan ikut salah.
    student_id    INTEGER     PRIMARY KEY REFERENCES students(id) ON DELETE RESTRICT,
    supervisor_id INTEGER     NOT NULL REFERENCES supervisors(id) ON DELETE RESTRICT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS supervisions_supervisor_idx ON supervisions (supervisor_id);

INSERT INTO supervisors (name, quota) VALUES
    ('Dosen Pembimbing A', 20),
    ('Dosen Pembimbing B', 20),
    ('Dosen Pembimbing C', 1),
    ('Dosen Rebutan',      3)
ON CONFLICT (name) DO NOTHING;

INSERT INTO permissions (name, description) VALUES
    ('supervision:manage', 'Menetapkan dan mengganti dosen pembimbing tugas akhir')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name) VALUES
    ('admin', 'supervision:manage')
ON CONFLICT DO NOTHING;

-- 30 mahasiswa khusus uji beban D.3. NIM berawalan 999 supaya mudah
-- dibedakan dan dibersihkan, dan tetap lolos CHECK students_nim_digits.
INSERT INTO students (nim, name, grade, is_active)
SELECT '999' || LPAD(g::TEXT, 6, '0'),
       'Mahasiswa Uji ' || LPAD(g::TEXT, 2, '0'),
       75, TRUE
FROM generate_series(1, 30) AS g
ON CONFLICT (nim) DO NOTHING;
