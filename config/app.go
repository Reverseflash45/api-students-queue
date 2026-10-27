package config

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/helper"
	"github.com/Reverseflash45/api-students-queue/middleware"
	"github.com/Reverseflash45/api-students-queue/route"
)

// NewApp merakit aplikasi: membuat instance Fiber, memasang middleware,
// lalu mendaftarkan route. Berkas ini adalah tempat seluruh bagian bertemu.
func NewApp(logger *slog.Logger, deps route.Dependencies) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      GetEnv("APP_NAME", "Praktikum Backend Lanjut - Pertemuan 7"),
		ErrorHandler: newErrorHandler(logger),

		// Membatasi ukuran body mencegah satu permintaan besar
		// menghabiskan memori server. Ini adalah denial of service yang
		// paling murah dilakukan: tidak perlu keahlian, tidak perlu akun,
		// cukup satu unggahan berukuran ratusan megabyte.
		//
		// 1 MB jauh lebih besar daripada body JSON mana pun di API ini -
		// batas yang longgar sekalipun sudah menutup serangannya.
		BodyLimit: 1 * 1024 * 1024,
	})

	middleware.Register(app, logger, GetEnv("ALLOWED_ORIGINS", ""))
	route.Register(app, deps)

	// Penampung terakhir untuk URL yang tidak dikenal. Ia pun hanya
	// MENGEMBALIKAN error, supaya bentuk responsnya ditulis oleh
	// ErrorHandler yang sama dengan kegagalan lainnya.
	app.Use(func(c *fiber.Ctx) error {
		return helper.NotFound("endpoint tidak ditemukan")
	})

	return app
}

// newErrorHandler adalah SATU-SATUNYA tempat error berubah menjadi
// response HTTP di seluruh aplikasi.
//
// Keuntungan yang tidak dapat dicapai dengan kedisiplinan semata:
// bentuk response kegagalan dijamin seragam, pencatatan log tidak dapat
// terlewat, dan detail teknis tidak dapat bocor karena pesan asli
// database hanya dibaca dari cause, yang tidak pernah ikut dikirim.
func newErrorHandler(logger *slog.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		requestID := helper.RequestID(c)

		var appErr *helper.AppError

		switch {
		case errors.As(err, &appErr):
			// Kegagalan yang sudah kita rencanakan.

		case errors.Is(err, fiber.ErrRequestEntityTooLarge):
			appErr = helper.PayloadTooLarge("ukuran body melebihi batas yang diizinkan")

		default:
			// Kegagalan yang tidak kita duga.
			var fiberErr *fiber.Error
			if errors.As(err, &fiberErr) {
				appErr = helper.FromFiberError(fiberErr)
			} else {
				appErr = helper.Internal(err)
			}
		}

		// Tingkat log ditentukan oleh SIAPA yang salah, bukan oleh
		// seberapa mengganggu kegagalannya. 4xx adalah kesalahan pemakai
		// API -> WARN. 5xx adalah kerusakan sistem kita sendiri -> ERROR.
		//
		// Bila tertukar, pemantauan akan berteriak setiap kali ada yang
		// salah ketik URL, dan diam saja ketika database benar-benar mati.
		if appErr.Status >= fiber.StatusInternalServerError {
			logger.Error("request_failed",
				slog.String("request_id", requestID),
				slog.String("path", c.Path()),
				slog.String("code", appErr.Code),
				slog.Int("status", appErr.Status),
				// appErr.Error(), bukan appErr.cause: field aslinya berhuruf
				// kecil sehingga tidak dapat disentuh package lain. Method
				// Error() ikut menyertakan cause bila ada, dan aman bila nil.
				slog.String("error", appErr.Error()))
		} else {
			logger.Warn("request_rejected",
				slog.String("request_id", requestID),
				slog.String("path", c.Path()),
				slog.String("code", appErr.Code),
				slog.Int("status", appErr.Status))
		}

		return c.Status(appErr.Status).JSON(model.ErrorResponse{
			Success:   false,
			Code:      appErr.Code,
			Message:   appErr.Message,
			Fields:    appErr.Fields,
			RequestID: requestID,
		})
	}
}
