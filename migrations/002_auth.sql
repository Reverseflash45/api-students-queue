-- ================================================================
-- MIGRASI 002: Authentication
--
-- CATATAN PENTING TERHADAP MODUL
-- Modul menyuruh "menambahkan column role pada table users". Perintah
-- itu mengandaikan project latihan-fiber yang sudah memiliki table
-- users sejak pertemuan sebelumnya.
--
-- Project ini berjalan di jalur yang berbeda: sejak pertemuan 3 yang
-- ada hanya table students, dan students BUKAN akun login, tidak
-- memiliki username maupun password. Karena itu table users dibuat
-- dari nol di sini, lengkap dengan column role.
--
-- Akibatnya, langkah "TRUNCATE users" pada modul tidak berlaku:
-- tidak ada password lama yang perlu dibuang karena belum pernah ada
-- password yang tersimpan. Alasan mengapa password lama tetap tidak
-- dapat diselamatkan dijelaskan pada laporan.
--
-- users   = siapa yang boleh masuk  (autentikasi)
-- students = data akademik          (domain aplikasi)
-- Keduanya sengaja TIDAK digabung.
-- ================================================================

CREATE TABLE IF NOT EXISTS users (
    id         SERIAL       PRIMARY KEY,
    username   VARCHAR(50)  NOT NULL,
    email      VARCHAR(255) NOT NULL,

    -- Panjang 255, bukan 60. Hash bcrypt saat ini 60 karakter, tetapi
    -- kolom yang pas-pasan akan memotong hash secara diam-diam bila
    -- kelak algoritmanya diganti (argon2id jauh lebih panjang).
    -- Hash yang terpotong tetap tersimpan dan selalu gagal dicocokkan.
    password   VARCHAR(255) NOT NULL,

    -- Role sudah disiapkan sekarang, tetapi belum dipakai untuk
    -- membatasi apa pun. Pembatasan hak akses dibahas pertemuan 6.
    role       VARCHAR(20)  NOT NULL DEFAULT 'user',

    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT users_username_length CHECK (LENGTH(TRIM(username)) >= 3),
    CONSTRAINT users_email_format    CHECK (POSITION('@' IN email) > 1),
    CONSTRAINT users_role_allowed    CHECK (role IN ('user', 'admin'))
);

-- LOWER() pada index membuat "Sari" dan "sari" dianggap satu username.
-- Tanpa ini, dua akun yang hanya berbeda huruf besar-kecil bisa lolos,
-- dan pemakai akan bingung akun mana yang benar.
-- Query login memakai LOWER(username) = LOWER($1) agar index ini terpakai.
CREATE UNIQUE INDEX IF NOT EXISTS users_username_key
    ON users (LOWER(username));

CREATE UNIQUE INDEX IF NOT EXISTS users_email_key
    ON users (LOWER(email));

-- ================================================================
-- REFRESH TOKEN
--
-- Yang disimpan adalah HASH-nya, bukan tokennya. Alasannya sama
-- persis dengan password: bila isi table ini bocor, penyerang hanya
-- memperoleh hash, bukan token yang dapat langsung dipakai.
--
-- Di sini SHA-256 sudah memadai dan bcrypt tidak diperlukan, karena
-- tokennya dibuat acak sepanjang 32 byte oleh crypto/rand, bukan
-- buatan manusia yang ruang tebakannya sempit.
-- ================================================================

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         BIGSERIAL   PRIMARY KEY,

    -- ON DELETE CASCADE: ketika user dihapus, seluruh token miliknya
    -- ikut terhapus. Tanpa ini, table menyimpan kredensial yang tidak
    -- berpemilik, bukan sekadar kotor, tetapi berbahaya.
    user_id    INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,

    -- NULL berarti masih aktif. Dipakai untuk rotasi dan logout.
    revoked_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS refresh_tokens_user_id_idx
    ON refresh_tokens (user_id);
