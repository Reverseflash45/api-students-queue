package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

var (
	ErrSupervisorFull       = errors.New("kuota pembimbing sudah penuh")
	ErrAlreadySupervised    = errors.New("mahasiswa sudah memiliki pembimbing")
	ErrNotSupervised        = errors.New("mahasiswa belum memiliki pembimbing")
	ErrStudentNotFound      = errors.New("mahasiswa tidak ditemukan")
	ErrQuotaBelowSupervised = errors.New("kuota lebih kecil dari jumlah bimbingan")
)

// SupervisorRepository mengikuti pola yang sama dengan CourseRepository:
// struct-nya menyimpan DBTX, bukan pool, sehingga seluruh method bisa
// dipindah ke dalam transaction lewat WithTx.
type SupervisorRepository interface {
	WithTx(tx DBTX) SupervisorRepository

	FindAll(ctx context.Context) ([]model.Supervisor, error)
	FindByID(ctx context.Context, id int) (model.Supervisor, error)
	FindSupervisorOf(ctx context.Context, studentID int) (int, error)

	LockStudent(ctx context.Context, studentID int) error
	LockSupervisors(ctx context.Context, ids ...int) error

	IncrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error)
	DecrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error)
	InsertSupervision(ctx context.Context, studentID, supervisorID int) error
	MoveSupervision(ctx context.Context, studentID, toSupervisorID int) error

	UpdateQuota(ctx context.Context, id, quota, version int) (model.Supervisor, error)
}

const supervisorColumns = `id, name, quota, supervised_count, version, created_at`

type supervisorPostgresRepository struct{ db DBTX }

func NewSupervisorRepository(db DBTX) SupervisorRepository {
	return &supervisorPostgresRepository{db: db}
}

func (r *supervisorPostgresRepository) WithTx(tx DBTX) SupervisorRepository {
	return &supervisorPostgresRepository{db: tx}
}

func scanSupervisor(row pgx.Row) (model.Supervisor, error) {
	var s model.Supervisor
	err := row.Scan(&s.ID, &s.Name, &s.Quota, &s.SupervisedCount, &s.Version, &s.CreatedAt)
	return s, err
}

func (r *supervisorPostgresRepository) FindAll(ctx context.Context) ([]model.Supervisor, error) {
	rows, err := r.db.Query(ctx, "SELECT "+supervisorColumns+" FROM supervisors ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar pembimbing: %w", err)
	}
	defer rows.Close()

	result := []model.Supervisor{}
	for rows.Next() {
		s, err := scanSupervisor(rows)
		if err != nil {
			return nil, fmt.Errorf("membaca row pembimbing: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *supervisorPostgresRepository) FindByID(ctx context.Context, id int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		"SELECT "+supervisorColumns+" FROM supervisors WHERE id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Supervisor{}, ErrNotFound
		}
		return model.Supervisor{}, fmt.Errorf("mengambil pembimbing: %w", err)
	}
	return s, nil
}

// FindSupervisorOf membaca pembimbing mahasiswa saat ini. Hasilnya hanya
// boleh dipakai untuk mengambil keputusan bila baris mahasiswa sudah
// dikunci lebih dulu dengan LockStudent.
func (r *supervisorPostgresRepository) FindSupervisorOf(ctx context.Context, studentID int) (int, error) {
	var supervisorID int
	err := r.db.QueryRow(ctx,
		"SELECT supervisor_id FROM supervisions WHERE student_id = $1", studentID).Scan(&supervisorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotSupervised
		}
		return 0, fmt.Errorf("membaca pembimbing mahasiswa: %w", err)
	}
	return supervisorID, nil
}

// LockStudent mengunci baris students milik satu mahasiswa.
//
// Selama kunci ini dipegang, transaction lain yang menyangkut mahasiswa
// yang SAMA (penetapan ke pembimbing lain, atau pergantian pembimbing)
// harus antre. Mahasiswa lain tidak ikut tertahan.
func (r *supervisorPostgresRepository) LockStudent(ctx context.Context, studentID int) error {
	var id int
	err := r.db.QueryRow(ctx,
		"SELECT id FROM students WHERE id = $1 FOR UPDATE", studentID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStudentNotFound
		}
		return fmt.Errorf("mengunci data mahasiswa: %w", err)
	}
	return nil
}

// LockSupervisors mengunci beberapa pembimbing dengan urutan id menaik,
// apa pun urutan argumennya. Urutan inilah yang membuat pergantian
// berlawanan arah (A ke B dan B ke A) tidak bisa saling menunggu.
func (r *supervisorPostgresRepository) LockSupervisors(ctx context.Context, ids ...int) error {
	rows, err := r.db.Query(ctx,
		"SELECT id FROM supervisors WHERE id = ANY($1) ORDER BY id FOR UPDATE", ids)
	if err != nil {
		return fmt.Errorf("mengunci pembimbing: %w", err)
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		found++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mengunci pembimbing: %w", err)
	}
	if found != len(ids) {
		return ErrNotFound
	}
	return nil
}

// IncrementSupervised mengambil satu slot bimbingan secara atomik.
// Pemeriksaan kuota dan penambahan terjadi dalam satu UPDATE.
func (r *supervisorPostgresRepository) IncrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET supervised_count = supervised_count + 1
		 WHERE id = $1 AND supervised_count < quota
		 RETURNING `+supervisorColumns, supervisorID))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Supervisor{}, fmt.Errorf("menambah bimbingan: %w", err)
	}

	if _, findErr := r.FindByID(ctx, supervisorID); findErr != nil {
		return model.Supervisor{}, findErr
	}
	return model.Supervisor{}, ErrSupervisorFull
}

func (r *supervisorPostgresRepository) DecrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET supervised_count = supervised_count - 1
		 WHERE id = $1
		 RETURNING `+supervisorColumns, supervisorID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Supervisor{}, ErrNotFound
		}
		return model.Supervisor{}, fmt.Errorf("mengurangi bimbingan: %w", err)
	}
	return s, nil
}

// InsertSupervision mencatat pasangan baru. student_id adalah PRIMARY KEY
// tabel supervisions, jadi mahasiswa yang sudah punya pembimbing akan
// ditolak database dengan 23505 dan kita terjemahkan ke ErrAlreadySupervised.
func (r *supervisorPostgresRepository) InsertSupervision(ctx context.Context, studentID, supervisorID int) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO supervisions (student_id, supervisor_id) VALUES ($1, $2)",
		studentID, supervisorID)
	if err != nil {
		if pgErrorCode(err) == codeUniqueViolation {
			return ErrAlreadySupervised
		}
		return fmt.Errorf("mencatat bimbingan: %w", err)
	}
	return nil
}

// MoveSupervision memindahkan mahasiswa ke pembimbing baru. Jumlah baris
// yang berubah tetap diperiksa: nol baris berarti mahasiswa itu tidak
// punya pembimbing, dan penghitung tidak boleh ikut diubah.
func (r *supervisorPostgresRepository) MoveSupervision(ctx context.Context, studentID, toSupervisorID int) error {
	tag, err := r.db.Exec(ctx,
		"UPDATE supervisions SET supervisor_id = $1, created_at = NOW() WHERE student_id = $2",
		toSupervisorID, studentID)
	if err != nil {
		return fmt.Errorf("memindah bimbingan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotSupervised
	}
	return nil
}

// UpdateQuota memakai optimistic locking, sama seperti kuota kelas.
func (r *supervisorPostgresRepository) UpdateQuota(ctx context.Context, id, quota, version int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET quota = $1, version = version + 1
		 WHERE id = $2 AND version = $3 AND $1 >= supervised_count
		 RETURNING `+supervisorColumns, quota, id, version))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Supervisor{}, fmt.Errorf("mengubah kuota pembimbing: %w", err)
	}

	current, findErr := r.FindByID(ctx, id)
	if findErr != nil {
		return model.Supervisor{}, findErr
	}
	if current.Version != version {
		return model.Supervisor{}, ErrVersionConflict
	}
	return model.Supervisor{}, ErrQuotaBelowSupervised
}
