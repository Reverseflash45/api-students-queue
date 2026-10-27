package helper

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// ================================================================
// Berkas ini berisi seluruh operasi kriptografi yang dipakai proyek.
// Dikumpulkan di satu tempat supaya mudah ditinjau: kode keamanan yang
// tersebar di banyak berkas adalah kode keamanan yang tidak pernah dibaca.
// ================================================================

// bcryptCost menentukan berapa kali proses hashing diulang secara internal.
// Nilai 12 berarti 2^12 = 4096 putaran.
//
// Mengapa 12, bukan 10 (nilai bawaan bcrypt) dan bukan 14:
//   - 10 sudah aman, tetapi kartu grafis modern semakin murah setiap tahun,
//     dan cost adalah satu-satunya tombol yang mengikuti perkembangan itu.
//   - 14 empat kali lebih lambat dari 12. Pada login tunggal selisihnya
//     masih wajar, tetapi endpoint login menjadi sasaran empuk denial of
//     service: setiap percobaan memaksa server bekerja keras.
//   - 12 adalah kompromi yang lazim dipakai: cukup lambat bagi penyerang
//     yang mencoba jutaan kemungkinan, masih cukup cepat untuk sekali login.
const bcryptCost = 12

// dummyHash adalah hash bcrypt yang sengaja tidak cocok dengan password
// mana pun. Dipakai ketika username tidak ditemukan.
//
// Tanpa ini, login dengan username yang tidak terdaftar akan menjawab
// jauh lebih cepat daripada login dengan password salah, karena jalur
// yang pertama tidak pernah menjalankan bcrypt sama sekali. Selisih waktu
// itu saja sudah cukup untuk memetakan username mana yang terdaftar
// (timing attack), meskipun pesan errornya sudah dibuat sama persis.
var dummyHash = []byte("$2a$12$abcdefghijklmnopqrstuuLKa3Bt1TCmU/6zvhZ8x4nq1yBiuGvS")

// HashPassword mengubah password menjadi hash yang tidak dapat dikembalikan.
//
// bcrypt membuat salt acak sendiri dan menyimpannya di dalam hasil hash,
// sehingga dua pemakai dengan password yang sama persis tetap menghasilkan
// hash yang berbeda. Itulah yang membuat rainbow table tidak berguna.
func HashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// VerifyPassword membandingkan password dengan hash-nya.
//
// Perhatikan: hash-nya tidak pernah "dibuka". bcrypt membaca cost dan salt
// dari dalam hash yang tersimpan, menghitung ulang dengan password yang
// baru diketik, lalu membandingkan hasilnya. Server tidak pernah tahu
// password aslinya, dan memang tidak perlu tahu.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// VerifyDummyPassword sengaja membuang waktu seperti VerifyPassword.
// Hasilnya dibuang karena memang tidak dipakai, yang dibutuhkan hanya
// waktu tempuhnya, agar mirip dengan jalur password salah.
func VerifyDummyPassword(plain string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(plain))
}

// RandomToken menghasilkan string acak yang aman secara kriptografis.
//
// crypto/rand, BUKAN math/rand. math/rand menghasilkan deret yang dapat
// diperkirakan bila seed-nya diketahui, dan seed yang lazim dipakai adalah
// waktu sekarang, sesuatu yang penyerang tahu persis.
func RandomToken(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// SHA256Hex dipakai untuk menyimpan refresh token dalam bentuk hash.
//
// Mengapa di sini SHA-256 boleh, sedangkan untuk password tidak:
// kelambatan bcrypt berguna melawan penebakan, dan penebakan hanya
// masuk akal bila ruang kemungkinannya sempit. Password buatan manusia
// ruangnya sempit; token 32 byte dari crypto/rand tidak. Memperlambat
// hashing token hanya memperlambat server sendiri.
func SHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
