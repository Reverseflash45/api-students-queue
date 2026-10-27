package helper

import (
	"testing"
	"time"
)

func TestEncodeDecodeCursorBolakBalik(t *testing.T) {
	waktu := time.Date(2026, 9, 23, 10, 30, 0, 123456789, time.UTC)

	cur, err := DecodeCursor(EncodeCursor(waktu, 42))
	if err != nil {
		t.Fatalf("decode gagal: %v", err)
	}
	if cur.ID != 42 {
		t.Fatalf("id = %d, mau 42", cur.ID)
	}
	if !cur.CreatedAt.Equal(waktu) {
		t.Fatalf("created_at = %v, mau %v", cur.CreatedAt, waktu)
	}
}

// Seluruh bentuk cursor yang rusak harus DITOLAK, bukan diam-diam
// dikembalikan ke halaman pertama.
func TestDecodeCursorRusak(t *testing.T) {
	rusak := []struct{ nama, nilai string }{
		{"kosong", ""},
		{"spasi", "   "},
		{"bukan base64", "bukanbase64!!"},
		{"base64 tanpa pemisah", "YWJjZGVm"},     // "abcdef"
		{"bagian waktu bukan angka", "YWJjfDEy"}, // "abc|12"
		{"id bukan angka", "MTIzfGFi"},           // "123|ab"
		{"id nol", "MTIzfDA"},                    // "123|0"
		{"tiga bagian", "MTIzfDF8OQ"},            // "123|1|9"
	}

	for _, c := range rusak {
		t.Run(c.nama, func(t *testing.T) {
			if _, err := DecodeCursor(c.nilai); err == nil {
				t.Fatalf("cursor %q seharusnya ditolak", c.nilai)
			}
		})
	}
}
