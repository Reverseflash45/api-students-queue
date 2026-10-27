package helper

import "testing"

type contohRequest struct {
	NIM      string  `json:"nim"       validate:"required,nim"`
	Nama     string  `json:"name"      validate:"required,min=3,max=100"`
	Nilai    float64 `json:"grade"     validate:"gte=0,lte=100"`
	NamaOpsi *string `json:"name_opsi,omitempty" validate:"omitnil,min=3"`
}

func ptr(s string) *string { return &s }

func TestValidateStruct(t *testing.T) {
	// Seluruh pelanggaran dilaporkan SEKALIGUS, bukan satu per satu.
	errs := ValidateStruct(contohRequest{NIM: "a b!", Nama: "ab", Nilai: 150})
	if len(errs) != 3 {
		t.Fatalf("mau 3 pelanggaran, dapat %d: %v", len(errs), errs)
	}
	if errs["nim"] == "" || errs["name"] == "" || errs["grade"] == "" {
		t.Fatalf("pesan per field tidak lengkap: %v", errs)
	}

	// Nama field memakai nama JSON, bukan nama Go.
	if _, ada := errs["NIM"]; ada {
		t.Fatal("pesan memakai nama Go, seharusnya nama JSON")
	}

	// Pesan tidak boleh kosong, pesan kosong berarti aturan yang
	// menolak justru menganggap nilainya sudah benar.
	for field, pesan := range errs {
		if pesan == "" {
			t.Fatalf("pesan untuk %q kosong", field)
		}
	}
}

func TestValidateStructValid(t *testing.T) {
	if errs := ValidateStruct(contohRequest{NIM: "434241117", Nama: "Rafi", Nilai: 88}); errs != nil {
		t.Fatalf("data sah seharusnya lolos, dapat %v", errs)
	}
}

// omitnil: field nil DILEWATI, field yang menunjuk string kosong DITOLAK.
func TestOmitnil(t *testing.T) {
	dasar := contohRequest{NIM: "434241117", Nama: "Rafi", Nilai: 88}

	if errs := ValidateStruct(dasar); errs != nil {
		t.Fatalf("field opsional nil seharusnya dilewati, dapat %v", errs)
	}

	dasar.NamaOpsi = ptr("")
	if errs := ValidateStruct(dasar); errs == nil {
		t.Fatal("pointer ke string kosong seharusnya DITOLAK")
	}
}

func TestPasswordStrength(t *testing.T) {
	cases := []struct{ password, mau string }{
		{"Rahasia123!", ""},
		{"abc", "minimal 8 karakter"},
		{"tanpaangka", "harus memuat huruf dan angka"},
		{"password123", "password terlalu umum"},
	}
	for _, c := range cases {
		if got := PasswordStrength(c.password); got != c.mau {
			t.Fatalf("PasswordStrength(%q) = %q, mau %q", c.password, got, c.mau)
		}
	}
}
