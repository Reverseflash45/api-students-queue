package model

import "time"

// ================================================================
// STRUCT REQUEST
// ================================================================

// RegisterRequest adalah bentuk body saat mendaftar.
//
// PERHATIKAN APA YANG TIDAK ADA: tidak ada field Role.
//
// Inilah penutup kerentanan mass assignment. Bila struct ini memuat
// Role. Siapa pun cukup menambahkan "role":"admin" pada body pendaftaran
// dan BodyParser akan mengisinya dengan patuh. Karena field-nya memang
// tidak ada, nilai itu diabaikan begitu saja, bukan divalidasi, bukan
// ditolak, melainkan tidak pernah punya tempat untuk masuk.
//
// Pertahanan yang paling andal adalah pertahanan yang tidak bergantung
// pada seseorang mengingat untuk memeriksanya.
type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=30,username"`
	Email    string `json:"email"    validate:"required,email,max=120"`
	Password string `json:"password" validate:"required,max=72,strongpassword"`
}

// Login hanya memeriksa KELENGKAPAN, bukan kekuatan password: aturan
// kekuatan berlaku saat password DIBUAT, bukan saat dipakai.
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ================================================================
// STRUCT RESPONSE
// ================================================================

// TokenPair adalah balasan login dan refresh.
//
// ExpiresIn dikirim dalam detik supaya client tahu kapan harus
// memperbarui token tanpa perlu membongkar isi JWT sendiri.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// ================================================================
// ENTITAS
// ================================================================

// RefreshToken adalah satu baris pada table refresh_tokens.
//
// Perhatikan: yang disimpan TokenHash, bukan tokennya sendiri. Struct ini
// sengaja tidak memiliki tag json karena memang tidak pernah dikirim
// keluar, nilainya hanya berputar antara service dan basis data.
type RefreshToken struct {
	ID        int64
	UserID    int
	TokenHash string
	ExpiresAt time.Time

	// Pointer, bukan time.Time biasa. nil berarti "belum pernah dicabut",
	// dan itu berbeda dari "dicabut pada waktu nol". Pola yang sama
	// dipakai pada PatchStudentRequest sejak pertemuan 2.
	RevokedAt *time.Time

	CreatedAt time.Time
}

// AuthUser adalah identitas yang dibawa access token dan disimpan di
// Locals. Isinya sengaja minimal: hanya yang benar-benar diperlukan
// middleware dan service, tidak lebih.
type AuthUser struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}
