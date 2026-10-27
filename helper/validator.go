package helper

import (
	"errors"
	"reflect"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// validate dibuat SEKALI untuk seluruh aplikasi.
//
// validator.New() melakukan refleksi dan menyimpan hasilnya dalam cache
// internal. Membuatnya ulang pada setiap request berarti membuang cache
// tersebut berkali-kali, mahal dan tidak ada gunanya.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()

	// Tanpa ini, pesan error menyebut nama field Go ("Username"),
	// padahal client mengirim dan membaca nama JSON ("username").
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return field.Name
		}
		return name
	})

	// Aturan buatan sendiri. Aturan yang tidak disediakan library tetap
	// ditulis secara deklaratif sebagai tag, bukan dikembalikan menjadi
	// pemeriksaan manual yang tersebar di dalam service.
	_ = v.RegisterValidation("nospace", func(fl validator.FieldLevel) bool {
		return !strings.ContainsAny(fl.Field().String(), " \t\n\r")
	})
	_ = v.RegisterValidation("username", func(fl validator.FieldLevel) bool {
		for _, r := range fl.Field().String() {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' {
				return false
			}
		}
		return true
	})
	// PERHATIKAN ARAH PERBANDINGANNYA. PasswordStrength mengembalikan
	// string KOSONG bila password memenuhi syarat, dan alasan penolakan
	// bila tidak. Aturan ini lolos ketika hasilnya kosong.
	_ = v.RegisterValidation("strongpassword", func(fl validator.FieldLevel) bool {
		return PasswordStrength(fl.Field().String()) == ""
	})
	// nim adalah custom validation milik domain students: hanya angka,
	// 9 sampai 12 digit. Ditulis sebagai tag supaya aturannya berada di
	// baris yang sama dengan definisi fieldnya.
	_ = v.RegisterValidation("nim", func(fl validator.FieldLevel) bool {
		nim := fl.Field().String()
		if len(nim) < 9 || len(nim) > 12 {
			return false
		}
		for _, r := range nim {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	})

	return v
}

// ValidateStruct menjalankan seluruh aturan pada tag struct dan
// mengembalikan peta nama field ke pesan berbahasa Indonesia.
// Mengembalikan nil berarti tidak ada pelanggaran.
func ValidateStruct(s any) map[string]string {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	// Terjadi bila yang dikirim bukan struct, itu kesalahan programmer,
	// bukan kesalahan pemakai API. Jangan diam-diam dianggap valid.
	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		return map[string]string{"_": "objek yang divalidasi tidak sah"}
	}

	var fieldErrors validator.ValidationErrors
	if !errors.As(err, &fieldErrors) {
		return map[string]string{"_": "validasi gagal"}
	}

	result := make(map[string]string, len(fieldErrors))
	for _, fe := range fieldErrors {
		key := fieldKey(fe)
		if _, exists := result[key]; !exists {
			result[key] = messageFor(fe)
		}
	}
	return result
}

// fieldKey memakai jalur lengkap field, misalnya "projects[1].title", supaya
// kesalahan pada elemen array yang berbeda tidak saling menimpa. Untuk field
// di tingkat teratas hasilnya sama dengan nama field biasa.
func fieldKey(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.Index(ns, "."); i >= 0 {
		return ns[i+1:]
	}
	return fe.Field()
}

// messageFor menerjemahkan nama tag menjadi kalimat yang dapat dibaca
// pemakai. Daftar ini terpusat: menambah satu tag baru cukup menambah
// satu case di sini, tidak menyebar ke banyak file.
func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "wajib diisi"
	case "email":
		return "format email tidak valid"
	case "min":
		switch fe.Kind() {
		case reflect.String:
			return "minimal " + fe.Param() + " karakter"
		case reflect.Slice, reflect.Array, reflect.Map:
			return "minimal " + fe.Param() + " item"
		}
		return "nilai minimal " + fe.Param()
	case "max":
		switch fe.Kind() {
		case reflect.String:
			return "maksimal " + fe.Param() + " karakter"
		case reflect.Slice, reflect.Array, reflect.Map:
			return "maksimal " + fe.Param() + " item"
		}
		return "nilai maksimal " + fe.Param()
	case "gte":
		return "nilai minimal " + fe.Param()
	case "lte":
		return "nilai maksimal " + fe.Param()
	case "alphanum":
		return "hanya boleh berisi huruf dan angka"
	case "nospace":
		return "tidak boleh mengandung spasi"
	case "username":
		return "hanya boleh huruf, angka, titik, dan garis bawah"
	case "nim":
		return "harus berisi 9 sampai 12 digit angka"
	case "strongpassword":
		// Type assertion memakai bentuk DUA nilai, bukan satu. Bentuk
		// satu nilai akan panic bila suatu saat tag ini terpasang pada
		// field bukan string, mematikan server hanya karena salah tag.
		if value, ok := fe.Value().(string); ok {
			return PasswordStrength(value)
		}
		return "password tidak memenuhi syarat"
	case "url":
		return "harus berupa URL yang valid, misalnya https://contoh.com"
	case "datetime":
		return "format tanggal harus " + fe.Param() + " (tahun-bulan-tanggal)"
	case "oneof":
		return "harus salah satu dari: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	default:
		// Jaring pengaman. Bila muncul di log, artinya ada tag yang
		// dipakai tetapi belum diterjemahkan di sini.
		return "tidak memenuhi aturan " + fe.Tag()
	}
}

const minPasswordLength = 8

// weakPasswords sengaja sangat pendek. Sistem sungguhan memakai daftar
// berisi jutaan password yang pernah bocor.
var weakPasswords = map[string]bool{
	"password1": true, "12345678": true, "qwerty123": true, "admin123": true,
	"password123": true, "rahasia1": true, "abcd1234": true,
}

// PasswordStrength dipindahkan apa adanya dari auth_rules.go milik Modul 5.
// Ia tetap berupa fungsi biasa, bukan sepenuhnya tag, karena sebuah tag
// hanya mampu menjawab benar atau salah, padahal "minimal 8 karakter"
// dan "password terlalu umum" menuntut tindakan berbeda dari pemakainya.
//
// String KOSONG berarti password memenuhi syarat.
func PasswordStrength(password string) string {
	if len(password) < minPasswordLength {
		return "minimal 8 karakter"
	}

	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}

	if !hasLetter || !hasDigit {
		return "harus memuat huruf dan angka"
	}

	if weakPasswords[strings.ToLower(password)] {
		return "password terlalu umum"
	}

	return ""
}
