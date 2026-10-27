package helper

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// Berkas ini berisi pembaca PERMINTAAN MASUK: mengubah bentuk mentah HTTP
// menjadi tipe milik proyek sendiri, sehingga service tidak perlu mengurus
// pembacaan query string maupun parameter jalur.

// RequestContext memberi batas waktu untuk setiap operasi basis data.
// Tanpa batas waktu, satu query yang menggantung dapat menahan koneksi
// selamanya dan lama-lama menghabiskan seluruh isi pool.
func RequestContext(c *fiber.Ctx) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.UserContext(), 5*time.Second)
}

// ParamID membaca parameter :id dari jalur dan memastikan bentuknya benar.
// Nilai yang bukan angka positif ditolak dengan 400, bukan 404, karena
// bentuk permintaannya memang salah.
func ParamID(c *fiber.Ctx) (int, bool) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

// allowedSort adalah daftar putih lapisan pertama, di sisi HTTP.
// Repository memiliki daftar putihnya sendiri sebagai lapisan kedua.
var allowedSort = map[string]bool{
	"id":         true,
	"nim":        true,
	"name":       true,
	"grade":      true,
	"created_at": true,
}

const (
	batasLimit  = 50
	limitBawaan = 10
)

// ParseListQuery membaca query string dan memberi nilai bawaan yang aman.
// Prinsipnya: masukan dari klien tidak pernah dipercaya begitu saja.
func ParseListQuery(c *fiber.Ctx) model.ListQuery {
	q := model.ListQuery{
		Page:   c.QueryInt("page", 1),
		Limit:  c.QueryInt("limit", limitBawaan),
		Search: strings.TrimSpace(c.Query("search")),
		Sort:   c.Query("sort", "id"),
		Order:  strings.ToLower(c.Query("order", "asc")),
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = limitBawaan
	}
	if q.Limit > batasLimit {
		q.Limit = batasLimit
	}
	if !allowedSort[q.Sort] {
		q.Sort = "id"
	}
	if q.Order != "desc" {
		q.Order = "asc"
	}

	if raw := c.Query("is_active"); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			q.IsActive = &v
		}
	}
	if raw := c.Query("min_grade"); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			q.MinGrade = &v
		}
	}
	if raw := c.Query("max_grade"); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			q.MaxGrade = &v
		}
	}

	return q
}
