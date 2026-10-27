package middleware

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/Reverseflash45/api-students-queue/helper"
)

// Register memasang seluruh middleware yang berlaku untuk semua route.
// URUTAN PENTING: middleware dieksekusi sesuai urutan pemasangan.
func Register(app *fiber.App, logger *slog.Logger, allowedOrigins string) {
	app.Use(requestid.New())            // 1. beri setiap permintaan satu ID unik
	app.Use(recover.New())              // 2. tangkap panic agar server tidak mati
	app.Use(helmet.New())               // 3. pasang header keamanan dasar
	app.Use(corsPolicy(allowedOrigins)) // 4. BERUBAH: tidak lagi cors.New()
	app.Use(RequestLogger(logger))      // 5. catat setiap permintaan
}

// corsPolicy membatasi origin yang boleh memanggil API.
//
// cors.New() tanpa konfigurasi mengizinkan SEMUA origin. Itu cukup untuk
// latihan pertemuan 2 ketika API-nya terbuka bagi siapa saja, tetapi
// tidak lagi memadai sekarang: begitu API membawa token, halaman mana pun
// di internet yang berhasil dibuka pemakai dapat memanggil API ini atas
// nama browser mereka.
//
// Daftar origin dibaca dari environment, bukan ditulis di dalam kode,
// karena alamatnya berbeda antara komputer sendiri dan server sungguhan.
func corsPolicy(allowedOrigins string) fiber.Handler {
	if strings.TrimSpace(allowedOrigins) == "" {
		allowedOrigins = "http://localhost:5173"
	}

	return cors.New(cors.Config{
		AllowOrigins: allowedOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",

		// Authorization WAJIB disebut. Header ini tidak termasuk daftar
		// bawaan CORS, sehingga tanpa baris ini browser akan memblokir
		// setiap permintaan yang membawa token, meski server sendiri
		// sebenarnya menerimanya.
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	})
}

// RequestLogger mencatat setiap permintaan ke log terstruktur.
//
// Perhatikan apa yang TIDAK dicatat: body permintaan dan isi header
// Authorization. Log adalah tempat kebocoran yang paling sering
// terlupakan, berkasnya dibaca banyak orang, disalin ke mana-mana, dan
// jarang dihapus.
func RequestLogger(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next() // serahkan ke middleware atau handler berikutnya

		requestID, _ := c.Locals("requestid").(string)

		// Sejak handler mengembalikan error alih-alih menulis response
		// sendiri, status pada c.Response() BELUM terisi ketika baris ini
		// dijalankan: ErrorHandler baru berjalan setelah seluruh rangkaian
		// middleware selesai. Tanpa koreksi di bawah, setiap kegagalan
		// tercatat sebagai 200, log menyesatkan tepat pada request yang
		// paling perlu ditelusuri.
		status := c.Response().StatusCode()
		if err != nil {
			var appErr *helper.AppError
			if errors.As(err, &appErr) {
				status = appErr.Status
			} else {
				status = fiber.StatusInternalServerError
			}
		}

		attrs := []any{
			slog.String("request_id", requestID),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", c.IP()),
		}

		// Identitas (user_id + role) ikut dicatat bila request sudah
		// melewati RequireAuth. Tanpa ini, log sebuah 403 tidak berguna:
		// kita tahu ada yang ditolak, tapi tidak tahu siapa dan kenapa.
		if user, ok := helper.CurrentUser(c); ok {
			attrs = append(attrs,
				slog.Int("user_id", user.UserID),
				slog.String("username", user.Username),
				slog.String("role", user.Role))
		}

		logger.Info("http_request", attrs...)

		return err
	}
}

var methodsWithBody = map[string]bool{
	fiber.MethodPost:  true,
	fiber.MethodPut:   true,
	fiber.MethodPatch: true,
}

// RequireJSON menolak permintaan berbody yang Content-Type-nya bukan JSON.
// Status yang tepat 415, bukan 400: bodynya mungkin benar, tetapi format
// yang dinyatakan klien tidak didukung endpoint ini.
func RequireJSON(c *fiber.Ctx) error {
	if methodsWithBody[c.Method()] {
		ct := c.Get("Content-Type")
		if !strings.HasPrefix(ct, fiber.MIMEApplicationJSON) {
			return helper.UnsupportedMediaType("Content-Type harus application/json")
		}
	}
	return c.Next()
}
