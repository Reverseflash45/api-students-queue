package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// ErrNotFound, ErrDuplicate, dan isUniqueViolation TIDAK dideklarasikan
// ulang di sini. Ketiganya sudah ada di student_repository.go, dan berkas
// ini berada di package yang sama.

// ================================================================
// KONTRAK
// Seperti StudentRepository: tidak ada kata SQL, postgres, maupun fiber.
// ================================================================

type UserRepository interface {
	Create(ctx context.Context, u model.User) (model.User, error)
	FindByID(ctx context.Context, id int) (model.User, error)
	FindByUsername(ctx context.Context, username string) (model.User, error)

	// Pertemuan 6
	FindAll(ctx context.Context) ([]model.User, error)
	FindAfterCursor(ctx context.Context, q model.CursorQuery) ([]model.User, error)
	UpdateRole(ctx context.Context, id int, role string) (model.User, error)
	Delete(ctx context.Context, id int) error
}

type userPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userPostgresRepository{pool: pool}
}

// kolomUser ditulis sekali agar urutan Scan tidak pernah berbeda
// antar query. Password ikut diambil karena Login memerlukannya untuk
// dicocokkan, yang mencegahnya bocor keluar adalah tag json:"-" pada
// struct User, bukan penghilangan kolom di sini.
const kolomUser = `id, username, email, password, role, is_active, created_at`

// ================================================================
// Create
// ================================================================

// Create menyimpan user baru.
//
// Role diambil dari struct yang dikirim service, dan service SELALU
// mengisinya dengan "user". Repository sengaja tidak memaksakan nilai itu
// sendiri supaya pertemuan 6 dapat membuat admin lewat jalur terpisah
// tanpa membongkar lapisan ini.
func (r *userPostgresRepository) Create(
	ctx context.Context, u model.User,
) (model.User, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password, role, is_active)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+kolomUser,
		u.Username, u.Email, u.Password, u.Role, u.IsActive,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role,
		&u.IsActive, &u.CreatedAt)

	if err != nil {
		// Pemeriksaan "apakah username sudah ada" TIDAK dilakukan dengan
		// SELECT lebih dulu. Antara SELECT dan INSERT ada jeda, dan dua
		// pendaftaran yang tiba bersamaan bisa sama-sama lolos pemeriksaan
		// (race condition). Yang menutup celah itu adalah UNIQUE INDEX di
		// basis data; tugas kode di sini hanya menerjemahkan penolakannya.
		if isUniqueViolation(err) {
			return model.User{}, ErrDuplicate
		}
		return model.User{}, fmt.Errorf("menyimpan user: %w", err)
	}

	return u, nil
}

// ================================================================
// FindByID
// ================================================================

func (r *userPostgresRepository) FindByID(
	ctx context.Context, id int,
) (model.User, error) {
	var u model.User

	err := r.pool.QueryRow(ctx,
		`SELECT `+kolomUser+` FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role,
		&u.IsActive, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, fmt.Errorf("mengambil user: %w", err)
	}

	return u, nil
}

// ================================================================
// FindByUsername
// ================================================================

// FindByUsername dipakai saat login.
//
// LOWER() di kedua sisi membuat pencocokan tidak membedakan huruf besar
// dan kecil, persis seperti UNIQUE INDEX users_username_key. Keduanya
// harus sama bentuknya: bila index memakai LOWER() sedangkan query tidak,
// PostgreSQL tidak dapat memakai index itu dan pencarian berubah menjadi
// pemindaian seluruh table.
func (r *userPostgresRepository) FindByUsername(
	ctx context.Context, username string,
) (model.User, error) {
	var u model.User

	err := r.pool.QueryRow(ctx,
		`SELECT `+kolomUser+` FROM users WHERE LOWER(username) = LOWER($1)`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role,
		&u.IsActive, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, fmt.Errorf("mengambil user: %w", err)
	}

	return u, nil
}

// ================================================================
// Pertemuan 6, FindAll, UpdateRole, Delete
// ================================================================

func (r *userPostgresRepository) FindAll(ctx context.Context) ([]model.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+kolomUser+` FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar user: %w", err)
	}
	defer rows.Close()

	hasil := []model.User{}
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role,
			&u.IsActive, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca baris user: %w", err)
		}
		hasil = append(hasil, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("membaca hasil query user: %w", err)
	}

	return hasil, nil
}

// UpdateRole sengaja dipisah dari perubahan data biasa. Mengubah role
// adalah tindakan istimewa yang dijaga permission tersendiri (role:assign).
func (r *userPostgresRepository) UpdateRole(
	ctx context.Context, id int, role string,
) (model.User, error) {
	var u model.User

	err := r.pool.QueryRow(ctx,
		`UPDATE users SET role = $1 WHERE id = $2 RETURNING `+kolomUser,
		role, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role,
		&u.IsActive, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, fmt.Errorf("mengubah role user: %w", err)
	}

	return u, nil
}

func (r *userPostgresRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("menghapus user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindAfterCursor: keyset pagination pada users. Penjelasan pasangan
// (created_at, id) sama persis dengan yang ada di student_repository.go.
func (r *userPostgresRepository) FindAfterCursor(
	ctx context.Context, q model.CursorQuery,
) ([]model.User, error) {
	args := []any{}
	where := " WHERE 1 = 1"

	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		where += fmt.Sprintf(" AND username ILIKE $%d", len(args))
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
		"SELECT %s FROM users%s ORDER BY created_at DESC, id DESC LIMIT $%d",
		kolomUser, where, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar user: %w", err)
	}
	defer rows.Close()

	result := []model.User{}
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role,
			&u.IsActive, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca row user: %w", err)
		}
		result = append(result, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("membaca hasil query: %w", err)
	}

	return result, nil
}
