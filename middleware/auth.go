package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/Reverseflash45/api-students-queue/helper"
)

// ================================================================
// RequireAuth: penjaga pintu
// ================================================================

// RequireAuth memeriksa access token pada header Authorization.
// Bila tokennya sah, identitas pemakai disimpan di Locals agar service
// dapat membacanya tanpa memeriksa ulang.
//
// Perhatikan polanya: function yang MENGEMBALIKAN fiber.Handler. Inilah
// cara middleware menerima dependensi dari luar, JWTManager diserahkan
// sekali saat pemasangan route, lalu ikut terbawa pada setiap request.
func RequireAuth(jwtManager *helper.JWTManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, err := bearerToken(c)
		if err != nil {
			// WWW-Authenticate adalah header baku yang WAJIB menyertai
			// 401 menurut RFC 9110. Fungsinya memberi tahu client
			// dengan cara apa ia seharusnya membuktikan identitasnya.
			c.Set(fiber.HeaderWWWAuthenticate, `Bearer realm="api"`)
			return helper.Unauthorized("header Authorization tidak ada atau salah bentuk")
		}

		authUser, err := jwtManager.Parse(token)
		if err != nil {
			c.Set(fiber.HeaderWWWAuthenticate, `Bearer realm="api"`)

			// ==========================================================
			// Di sini "kedaluwarsa" DIBEDAKAN dari "tidak valid",
			// padahal pada login kedua jalur kegagalan justru disamakan.
			// Perbedaan perlakuan itu disengaja, dan alasannya begini:
			//
			// Pada login, yang dibedakan adalah dua RAHASIA, apakah
			// username itu terdaftar. Pengirim pesan belum membuktikan
			// apa pun, jadi setiap perbedaan jawaban adalah informasi
			// gratis bagi orang yang sedang menebak.
			//
			// Pada token, yang dibedakan adalah keadaan SESUATU YANG
			// SUDAH DIPEGANG client. Pemegang token yang sah memang
			// perlu tahu kapan harus memanggil /auth/refresh, sedangkan
			// bagi penyerang informasi "token ini kedaluwarsa" tidak
			// menambah apa pun: ia tetap tidak bisa membuat token baru
			// tanpa secret, dan token kedaluwarsa tetap ditolak.
			// ==========================================================
			if errors.Is(err, helper.ErrExpiredToken) {
				return helper.Unauthorized("access token kedaluwarsa")
			}
			return helper.Unauthorized("access token tidak valid")
		}

		c.Locals(helper.LocalsAuthUser, authUser)
		return c.Next()
	}
}

// bearerToken membaca token dari header "Authorization: Bearer <token>".
//
// Bentuknya diperiksa ketat karena header inilah pintu masuk satu-satunya.
// EqualFold dipakai untuk "Bearer" karena skema autentikasi tidak peka
// huruf besar-kecil menurut RFC 9110, tetapi tokennya sendiri peka.
func bearerToken(c *fiber.Ctx) (string, error) {
	header := c.Get(fiber.HeaderAuthorization)
	if header == "" {
		return "", errors.New("header kosong")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("format bukan Bearer")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("token kosong")
	}

	return token, nil
}

// ================================================================
// LoginRateLimiter: penutup brute force
// ================================================================

// LoginRateLimiter membatasi jumlah percobaan login dari satu alamat IP.
//
// Tanpa pembatasan ini, penyerang dapat mencoba ribuan password per menit
// tanpa hambatan apa pun. bcrypt cost 12 memperlambatnya, tetapi
// memperlambat bukan menghentikan.
//
// BATASNYA: pembatasan per IP dapat dilewati dengan mengganti-ganti IP,
// dan sebaliknya dapat ikut menghukum banyak pemakai yang berbagi satu IP
// publik, misalnya seluruh kampus di balik satu NAT. Sistem sungguhan
// menggabungkannya dengan pembatasan per akun dan penundaan bertingkat.
func LoginRateLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,

		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},

		LimitReached: func(c *fiber.Ctx) error {
			// Retry-After memberi tahu client berapa detik lagi ia boleh
			// mencoba, sehingga client yang tertib tidak perlu menebak.
			c.Set("Retry-After", "60")
			return helper.TooManyRequests(
				"terlalu banyak percobaan login, coba lagi dalam satu menit")
		},
	})
}
