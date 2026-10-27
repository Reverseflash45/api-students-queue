package helper

import (
	"encoding/csv"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

const (
	FormatJSON = fiber.MIMEApplicationJSON
	FormatCSV  = "text/csv"
)

// Negotiate memilih format response berdasarkan header Accept.
//
// Perbedaan yang wajib jelas:
//   - Content-Type menjelaskan format yang SEDANG DIKIRIM pengirim.
//   - Accept menjelaskan format yang DIINGINKAN penerima sebagai balasan.
func Negotiate(c *fiber.Ctx, offered ...string) (string, error) {
	accept := strings.TrimSpace(c.Get(fiber.HeaderAccept))

	// Tidak menyebut Accept sama sekali berarti "terserah server".
	// Demikian pula Accept: */* yang dikirim hampir semua tool CLI.
	if accept == "" || accept == "*/*" {
		return offered[0], nil
	}

	chosen := c.Accepts(offered...)
	if chosen == "" {
		return "", NotAcceptable(
			"format yang diminta tidak tersedia, pilih salah satu dari: " +
				strings.Join(offered, ", "))
	}

	return chosen, nil
}

// writeCSV menuliskan satu tabel CSV beserta headernya.
//
// Content-Disposition membuat browser menawarkan unduhan alih-alih
// menampilkan isinya sebagai teks mentah.
//
// writer.Flush() WAJIB dipanggil sebelum buffer dibaca: csv.Writer
// menahan tulisannya di buffer internal, sehingga tanpa Flush berkas
// yang terkirim berukuran nol tanpa satu pun pesan kesalahan.
func writeCSV(c *fiber.Ctx, namaBerkas string, header []string, rows [][]string) error {
	c.Set(fiber.HeaderContentType, FormatCSV+"; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+namaBerkas+`"`)

	var buffer strings.Builder
	writer := csv.NewWriter(&buffer)

	if err := writer.Write(header); err != nil {
		return Internal(err)
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return Internal(err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return Internal(err)
	}

	return c.SendString(buffer.String())
}

// WriteUsersCSV menuliskan daftar user sebagai CSV.
func WriteUsersCSV(c *fiber.Ctx, users []model.User) error {
	rows := make([][]string, 0, len(users))
	for _, u := range users {
		rows = append(rows, []string{
			strconv.Itoa(u.ID), u.Username, u.Email, u.Role,
			strconv.FormatBool(u.IsActive),
			u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return writeCSV(c, "users.csv",
		[]string{"id", "username", "email", "role", "is_active", "created_at"}, rows)
}

// WriteStudentsCSV menuliskan daftar mahasiswa sebagai CSV.
//
// owner_id ikut ditulis karena ia bagian dari data; nilai NULL
// (data tak bertuan warisan Modul 6) ditulis sebagai sel kosong.
func WriteStudentsCSV(c *fiber.Ctx, students []model.Student) error {
	rows := make([][]string, 0, len(students))
	for _, s := range students {
		owner := ""
		if s.OwnerID != nil {
			owner = strconv.Itoa(*s.OwnerID)
		}
		rows = append(rows, []string{
			strconv.Itoa(s.ID), s.NIM, s.Name,
			strconv.FormatFloat(s.Grade, 'f', 2, 64),
			strconv.FormatBool(s.IsActive), owner,
			s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return writeCSV(c, "students.csv",
		[]string{"id", "nim", "name", "grade", "is_active", "owner_id", "created_at"}, rows)
}
