package model

import "time"

// ================================================================
// User adalah AKUN LOGIN, bukan data akademik.
//
// Perbedaan ini sengaja dipertahankan: Student menjawab "siapa mahasiswa
// ini dan berapa nilainya", User menjawab "siapa yang sedang memakai API
// ini". Menggabungkan keduanya terasa hemat pada awalnya, tetapi berarti
// setiap mahasiswa wajib punya password dan setiap admin wajib punya NIM.
// ================================================================

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`

	// json:"-" adalah penutup kerentanan "password bocor lewat response".
	// Tanpa tag ini, setiap endpoint yang mengembalikan User, register,
	// /auth/me, daftar user, ikut mengirimkan hash password-nya.
	// Satu tag di satu tempat menutup seluruh jalur sekaligus, jauh lebih
	// aman daripada mengingat untuk menghapusnya di setiap handler.
	Password string `json:"-"`

	// Sejak pertemuan 6, Role dipakai untuk authorization (RBAC):
	// nilainya dijaga FOREIGN KEY ke table roles.
	Role string `json:"role"`

	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// AssignRoleRequest dipakai endpoint PATCH /users/:id/role (pertemuan 6).
type AssignRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=admin staff user"`
}
