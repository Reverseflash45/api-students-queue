package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// ================================================================
// SENTINEL ERROR
// Error milik lapisan repository, bukan error milik pgx.
// Lapisan atas cukup mengenal dua ini dan tidak perlu tahu basis datanya apa.
// Kalau kelak pindah ke MySQL atau MongoDB, handler tidak perlu diubah.
// ================================================================

var (
	ErrNotFound  = errors.New("data tidak ditemukan")
	ErrDuplicate = errors.New("data sudah ada")
)

// ================================================================
// KONTRAK
// Perhatikan: tidak ada satu pun kata "SQL", "postgres", atau "fiber" di sini.
// ================================================================

type StudentRepository interface {
	FindAll(ctx context.Context, q model.ListQuery) ([]model.Student, int, error)
	FindByID(ctx context.Context, id int) (model.Student, error)
	Create(ctx context.Context, s model.Student) (model.Student, error)
	Update(ctx context.Context, s model.Student) (model.Student, error)
	Delete(ctx context.Context, id int) error

	// FindAfterCursor mengambil satu halaman memakai keyset pagination.
	FindAfterCursor(ctx context.Context, q model.CursorQuery) ([]model.Student, error)

	// FindOwnerID hanya membaca owner_id, dipakai pemeriksaan hak akses
	// SEBELUM data lengkap diambil. Mengembalikan 0 bila tak bertuan.
	FindOwnerID(ctx context.Context, id int) (int, error)
}

// kolomUrut adalah daftar putih: pemetaan dari nilai yang boleh dikirim klien
// ke nama kolom yang sebenarnya.
//
// ORDER BY tidak dapat memakai parameter ($1, $2, ...) karena nama kolom
// bukan nilai. Nama kolom terpaksa disisipkan sebagai teks, dan daftar putih
// inilah SATU-SATUNYA hal yang mencegah SQL injection di titik ini.
var kolomUrut = map[string]string{
	"id":         "id",
	"nim":        "nim",
	"name":       "name",
	"grade":      "grade",
	"created_at": "created_at",
}

// kolomBawaan dipakai bila nilai sort dari klien tidak ada di daftar putih.
const kolomBawaan = "id"

type studentPostgresRepository struct {
	pool *pgxpool.Pool
}

// NewStudentRepository mengembalikan INTERFACE, bukan struct konkret.
// Pemanggil hanya melihat kontraknya, bukan implementasinya.
func NewStudentRepository(pool *pgxpool.Pool) StudentRepository {
	return &studentPostgresRepository{pool: pool}
}

// kolomPilihan adalah daftar kolom yang selalu diambil, ditulis sekali
// agar urutan Scan tidak pernah beda antar query.
const kolomPilihan = `id, nim, name, grade, is_active, created_at, owner_id`

// ================================================================
// PENYUSUN KLAUSA WHERE
// ================================================================

// buildFilter menyusun bagian WHERE beserta argumennya.
// Nilai dari klien SELALU menjadi argumen ($1, $2, ...), tidak pernah
// disambung langsung ke dalam teks SQL.
//
// "WHERE 1 = 1" dipakai sebagai titik awal supaya setiap syarat berikutnya
// bisa ditambahkan dengan " AND ..." tanpa perlu memeriksa apakah dia yang
// pertama.
func buildFilter(q model.ListQuery) (string, []any) {
	where := " WHERE 1 = 1"
	args := []any{}

	if q.Search != "" {
		// ILIKE = LIKE yang tidak membedakan huruf besar dan kecil.
		// Tanda persen dipasang pada NILAI argumennya, bukan pada teks SQL,
		// sehingga isi pencarian tetap diperlakukan sebagai data.
		where += fmt.Sprintf(" AND name ILIKE $%d", len(args)+1)
		args = append(args, "%"+q.Search+"%")
	}

	if q.IsActive != nil {
		where += fmt.Sprintf(" AND is_active = $%d", len(args)+1)
		args = append(args, *q.IsActive)
	}

	if q.MinGrade != nil {
		where += fmt.Sprintf(" AND grade >= $%d", len(args)+1)
		args = append(args, *q.MinGrade)
	}

	if q.MaxGrade != nil {
		where += fmt.Sprintf(" AND grade <= $%d", len(args)+1)
		args = append(args, *q.MaxGrade)
	}

	return where, args
}

// ================================================================
// FindAll: penyaringan, pengurutan, dan paginasi seluruhnya di SQL
// ================================================================

func (r *studentPostgresRepository) FindAll(
	ctx context.Context, q model.ListQuery,
) ([]model.Student, int, error) {
	where, args := buildFilter(q)

	// 1) Hitung total SEBELUM dipenggal, untuk keperluan meta.
	//    Syarat penyaringannya harus sama persis dengan query pengambilan,
	//    kalau tidak, meta.total akan berbohong.
	var total int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM students"+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("menghitung student: %w", err)
	}

	// 2) Ambil satu halaman saja. Pada pertemuan 2 seluruh data diambil
	//    lebih dulu lalu disaring di Go; sekarang basis data yang
	//    mengerjakannya, dan yang dikirim lewat jaringan hanya satu halaman.
	kolom, ada := kolomUrut[q.Sort]
	if !ada {
		kolom = kolomBawaan
	}

	arah := "ASC"
	if q.Order == "desc" {
		arah = "DESC"
	}

	sqlText := fmt.Sprintf(
		`SELECT %s
		 FROM students%s
		 ORDER BY %s %s
		 LIMIT $%d OFFSET $%d`,
		kolomPilihan, where, kolom, arah, len(args)+1, len(args)+2,
	)
	args = append(args, q.Limit, q.Offset())

	rows, err := r.pool.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mengambil daftar student: %w", err)
	}
	// Selama rows belum ditutup, koneksi yang dipakainya belum kembali ke pool.
	// Melupakan baris ini beberapa kali sudah cukup untuk menghabiskan pool,
	// dan gejalanya adalah aplikasi yang berhenti melayani tanpa pesan apa pun.
	defer rows.Close()

	hasil := []model.Student{}
	for rows.Next() {
		var s model.Student
		if err := rows.Scan(&s.ID, &s.NIM, &s.Name, &s.Grade,
			&s.IsActive, &s.CreatedAt, &s.OwnerID); err != nil {
			return nil, 0, fmt.Errorf("membaca baris student: %w", err)
		}
		hasil = append(hasil, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("membaca hasil query: %w", err)
	}

	return hasil, total, nil
}

// ================================================================
// FindByID
// ================================================================

func (r *studentPostgresRepository) FindByID(
	ctx context.Context, id int,
) (model.Student, error) {
	var s model.Student

	err := r.pool.QueryRow(ctx,
		`SELECT `+kolomPilihan+` FROM students WHERE id = $1`, id,
	).Scan(&s.ID, &s.NIM, &s.Name, &s.Grade, &s.IsActive, &s.CreatedAt, &s.OwnerID)

	if err != nil {
		// pgx.ErrNoRows diterjemahkan menjadi error milik kita sendiri,
		// supaya handler tidak perlu mengenal pgx sama sekali.
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Student{}, ErrNotFound
		}
		return model.Student{}, fmt.Errorf("mengambil student: %w", err)
	}

	return s, nil
}

// ================================================================
// Create
// ================================================================

func (r *studentPostgresRepository) Create(
	ctx context.Context, s model.Student,
) (model.Student, error) {
	// RETURNING membuat id dan created_at hasil buatan basis data
	// langsung ikut kembali, tanpa perlu query kedua.
	err := r.pool.QueryRow(ctx,
		`INSERT INTO students (nim, name, grade, is_active, owner_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		s.NIM, s.Name, s.Grade, s.IsActive, s.OwnerID,
	).Scan(&s.ID, &s.CreatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return model.Student{}, ErrDuplicate
		}
		return model.Student{}, fmt.Errorf("menyimpan student: %w", err)
	}

	return s, nil
}

// ================================================================
// Update
// ================================================================

func (r *studentPostgresRepository) Update(
	ctx context.Context, s model.Student,
) (model.Student, error) {
	// owner_id SENGAJA tidak ada di SET: kepemilikan tidak bisa dipindah
	// lewat PUT/PATCH, bahkan oleh admin.
	//
	// RETURNING mengembalikan baris hasil perubahan dalam satu perjalanan,
	// sehingga field yang tidak ikut diubah (created_at) tetap terisi benar.
	err := r.pool.QueryRow(ctx,
		`UPDATE students
		 SET nim = $1, name = $2, grade = $3, is_active = $4
		 WHERE id = $5
		 RETURNING `+kolomPilihan,
		s.NIM, s.Name, s.Grade, s.IsActive, s.ID,
	).Scan(&s.ID, &s.NIM, &s.Name, &s.Grade, &s.IsActive, &s.CreatedAt, &s.OwnerID)

	if err != nil {
		// Tidak ada baris yang dikembalikan berarti id-nya memang tidak ada.
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Student{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return model.Student{}, ErrDuplicate
		}
		return model.Student{}, fmt.Errorf("memperbarui student: %w", err)
	}

	return s, nil
}

// ================================================================
// FindOwnerID
// ================================================================

func (r *studentPostgresRepository) FindOwnerID(ctx context.Context, id int) (int, error) {
	var owner int

	// COALESCE: data lama tak bertuan (NULL) dibaca sebagai 0.
	// Id user dimulai dari 1, jadi 0 tidak pernah cocok dengan siapa pun.
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(owner_id, 0) FROM students WHERE id = $1`, id,
	).Scan(&owner)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("mengambil pemilik student: %w", err)
	}

	return owner, nil
}

// ================================================================
// Delete
// ================================================================

func (r *studentPostgresRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM students WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("menghapus student: %w", err)
	}

	// Perintah berhasil dijalankan, tetapi tidak ada baris yang terkena.
	// Artinya id-nya memang tidak ada.
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ================================================================
// BANTUAN
// ================================================================

// isUniqueViolation memeriksa apakah error berasal dari pelanggaran
// batasan UNIQUE. Kode 23505 adalah kode resmi PostgreSQL untuk itu.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// ================================================================
// FindAfterCursor: keyset pagination
// ================================================================

// FindAfterCursor mengambil satu halaman memakai keyset pagination.
//
// Pasangan pengurutnya (created_at, id). id ikut dibandingkan karena
// created_at TIDAK dijamin unik: dua baris dapat dibuat pada mikrodetik
// yang sama, dan bila hanya created_at yang dibandingkan salah satunya
// akan terlewat atau terkirim dua kali. id adalah primary key, sehingga
// pasangan itu pasti unik.
//
// (created_at, id) < ($1, $2) adalah row value comparison: PostgreSQL
// membandingkannya secara leksikografis, persis seperti ORDER BY
// created_at DESC, id DESC mengurutkannya, dan bentuk ini dapat langsung
// memakai index gabungan.
//
// Jumlah yang diminta sengaja limit+1. Baris tambahan itu tidak dikirim
// ke client; keberadaannya hanya dipakai untuk menjawab "masih ada
// halaman berikutnya?" tanpa perlu COUNT(*) atas seluruh tabel.
func (r *studentPostgresRepository) FindAfterCursor(
	ctx context.Context, q model.CursorQuery,
) ([]model.Student, error) {
	args := []any{}
	where := " WHERE 1 = 1"

	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		where += fmt.Sprintf(" AND name ILIKE $%d", len(args))
	}
	if q.IsActive != nil {
		args = append(args, *q.IsActive)
		where += fmt.Sprintf(" AND is_active = $%d", len(args))
	}
	if q.After != nil {
		args = append(args, q.After.CreatedAt, q.After.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}

	args = append(args, q.Limit+1)
	query := fmt.Sprintf(
		"SELECT %s FROM students%s ORDER BY created_at DESC, id DESC LIMIT $%d",
		kolomPilihan, where, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar student: %w", err)
	}
	defer rows.Close()

	result := []model.Student{}
	for rows.Next() {
		var s model.Student
		if err := rows.Scan(&s.ID, &s.NIM, &s.Name, &s.Grade,
			&s.IsActive, &s.CreatedAt, &s.OwnerID); err != nil {
			return nil, fmt.Errorf("membaca row student: %w", err)
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("membaca hasil query: %w", err)
	}

	return result, nil
}
