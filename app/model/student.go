package model

import (
	"fmt"
	"time"
)

// ================================================================
// ENTITAS
// ================================================================

// Student adalah struct dari pertemuan 1, ditambah NIM sebagai penanda unik.
// Sejak pertemuan 3, seluruh field ini berasal dari kolom tabel students.
type Student struct {
	ID        int       `json:"id"`
	NIM       string    `json:"nim"`
	Name      string    `json:"name"`
	Grade     float64   `json:"grade"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`

	// OwnerID = user yang mendaftarkan data ini (pertemuan 6).
	// Pointer karena data lama bernilai NULL (tak bertuan).
	//
	// PERHATIKAN: field ini TIDAK ADA di Create/Replace/PatchStudentRequest.
	// Jadi "owner_id" yang dikirim lewat body tidak punya tempat untuk
	// masuk, nilainya diisi service dari token, bukan dari klien.
	OwnerID *int `json:"owner_id"`
}

// ================================================================
// METHOD: dibawa dari pertemuan 1
// ================================================================

// GetInfo memakai VALUE receiver karena hanya membaca field.
func (s Student) GetInfo() string {
	status := "Tidak Aktif"
	if s.IsActive {
		status = "Aktif"
	}
	return fmt.Sprintf("[%s] %s | Nilai: %.2f (%s) | Status: %s",
		s.NIM, s.Name, s.Grade, s.Huruf(), status)
}

// Huruf mengubah nilai angka menjadi nilai huruf. Value receiver, hanya membaca.
func (s Student) Huruf() string {
	switch {
	case s.Grade >= 85:
		return "A"
	case s.Grade >= 75:
		return "B"
	case s.Grade >= 65:
		return "C"
	case s.Grade >= 55:
		return "D"
	default:
		return "E"
	}
}

// UpdateGrade memakai POINTER receiver karena mengubah isi struct.
func (s *Student) UpdateGrade(grade float64) { s.Grade = grade }

// Activate dan Deactivate juga pointer receiver, alasan sama: mengubah state.
func (s *Student) Activate()   { s.IsActive = true }
func (s *Student) Deactivate() { s.IsActive = false }

// ================================================================
// STRUCT REQUEST, dipisah per metode HTTP
// ================================================================

// CreateStudentRequest untuk POST. Semua field wajib diisi.
// Sengaja TANPA OwnerID (penutup mass assignment, sama seperti Role
// pada RegisterRequest di pertemuan 5).
type CreateStudentRequest struct {
	NIM   string  `json:"nim"   validate:"required,nim"`
	Name  string  `json:"name"  validate:"required,min=3,max=100"`
	Grade float64 `json:"grade" validate:"gte=0,lte=100"`
}

// ReplaceStudentRequest untuk PUT. Mengganti seluruh isi, jadi field bertipe
// biasa dan semuanya wajib dikirim.
type ReplaceStudentRequest struct {
	NIM      string  `json:"nim"       validate:"required,nim"`
	Name     string  `json:"name"      validate:"required,min=3,max=100"`
	Grade    float64 `json:"grade"     validate:"gte=0,lte=100"`
	IsActive bool    `json:"is_active"`
}

// PatchStudentRequest untuk PATCH. Field bertipe POINTER supaya server bisa
// membedakan "tidak dikirim" (nil) dari "dikirim bernilai kosong/nol".
// Pada PATCH, pointer membedakan "tidak dikirim" (nil) dari "dikirim
// bernilai kosong". omitnil dipilih, bukan omitempty, karena ia
// menyatakan maksud yang sebenarnya: lewati HANYA bila nil. Dengan
// omitempty pada field bukan pointer, {"name":""} akan lolos tanpa satu
// pun aturan dijalankan.
type PatchStudentRequest struct {
	NIM      *string  `json:"nim,omitempty"       validate:"omitnil,nim"`
	Name     *string  `json:"name,omitempty"      validate:"omitnil,min=3,max=100"`
	Grade    *float64 `json:"grade,omitempty"     validate:"omitnil,gte=0,lte=100"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// ================================================================
// AMPLOP RESPONS, dipakai seragam oleh SELURUH endpoint
// ================================================================

// ErrorResponse adalah bentuk TUNGGAL seluruh response kegagalan sejak
// pertemuan 7. Field "errors" milik Modul 6 berganti menjadi "fields",
// dan muncul "code" serta "request_id", perubahan yang di dunia nyata
// menuntut versi baru (/api/v2), dan itulah alasan URL kita sejak awal
// memuat /v1.
type ErrorResponse struct {
	Success   bool              `json:"success"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// Cursor adalah penanda posisi keyset pagination. created_at saja tidak
// cukup karena ia tidak dijamin unik; id menjadi pemecah serinya.
type Cursor struct {
	CreatedAt time.Time
	ID        int
}

// CursorQuery menampung query string endpoint bercursor.
type CursorQuery struct {
	Limit    int
	Search   string
	IsActive *bool
	After    *Cursor
}

// CursorMeta menggantikan Meta pada endpoint yang memakai cursor.
//
// Perhatikan tidak adanya Total dan TotalPages. Keduanya tidak dapat
// disediakan tanpa COUNT(*) atas seluruh tabel, persis biaya yang ingin
// dihindari oleh pagination berbasis cursor.
type CursorMeta struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type CursorResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    any         `json:"data,omitempty"`
	Meta    *CursorMeta `json:"meta,omitempty"`
}

type WebResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

type Meta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// ListQuery menampung hasil pembacaan query string yang sudah dibersihkan.
// Field filter memakai pointer: nil berarti filter tidak dipakai sama sekali.
type ListQuery struct {
	Page     int
	Limit    int
	Search   string
	Sort     string
	Order    string
	IsActive *bool
	MinGrade *float64
	MaxGrade *float64
}

// Offset menghitung berapa baris yang dilewati untuk halaman ini.
// Perhitungan ini berada di sini karena kini dipakai langsung oleh SQL
// pada klausa OFFSET.
func (q ListQuery) Offset() int {
	return (q.Page - 1) * q.Limit
}
