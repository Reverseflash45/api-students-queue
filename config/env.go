package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// LoadEnv memuat variabel dari berkas .env ke environment proses.
// Bila berkasnya tidak ada, aplikasi tetap jalan memakai environment sistem.
// Ini penting untuk deployment: di server, nilai biasanya diberikan lewat
// environment, bukan lewat berkas.
func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("peringatan: berkas .env tidak ditemukan, memakai environment sistem")
	}
}

// GetEnv mengambil nilai environment. Bila kosong, kembalikan nilai bawaan.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

// GetEnvInt sama seperti GetEnv, tetapi hasilnya berupa int.
// Nilai yang tidak bisa dibaca sebagai angka tidak membuat aplikasi berhenti,
// melainkan dikembalikan ke nilai bawaan disertai peringatan.
func GetEnvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("peringatan: %s bukan angka (%q), memakai bawaan %d", key, value, fallback)
		return fallback
	}
	return parsed
}
