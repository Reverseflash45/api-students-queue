package helper

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// Paket helper berperan sebagai PRESENTER: satu-satunya tempat yang
// menyusun bentuk respons keluar. Service memutuskan APA yang dikirim,
// helper memutuskan BAGAIMANA bentuknya.
//
// Perhatikan huruf besar di awal nama fungsi. Karena helper kini menjadi
// package tersendiri, fungsi yang dipakai dari luar harus diawali huruf
// kapital, aturan visibilitas Go dari pertemuan 1, kini terasa akibatnya.

func Success(c *fiber.Ctx, status int, message string, data any) error {
	return c.Status(status).JSON(model.WebResponse{
		Success: true, Message: message, Data: data,
	})
}

func SuccessList(c *fiber.Ctx, message string, data any, meta *model.Meta) error {
	return c.Status(fiber.StatusOK).JSON(model.WebResponse{
		Success: true, Message: message, Data: data, Meta: meta,
	})
}

// Created mengirim 201 sekaligus memasang header Location.
func Created(c *fiber.Ctx, message string, data any, location string) error {
	c.Set("Location", location)
	return c.Status(fiber.StatusCreated).JSON(model.WebResponse{
		Success: true, Message: message, Data: data,
	})
}

// NoContent mengirim 204 tanpa body, sesuai spesifikasi HTTP.
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// SuccessCursor membalas daftar yang memakai cursor pagination.
// Bentuk metanya berbeda dari Meta biasa: tanpa total dan total_pages.
func SuccessCursor(c *fiber.Ctx, message string, data any, meta *model.CursorMeta) error {
	return c.Status(fiber.StatusOK).JSON(model.CursorResponse{
		Success: true, Message: message, Data: data, Meta: meta,
	})
}

// RequestID membaca id yang dipasang middleware requestid.
// Dipakai ErrorHandler agar satu keluhan pemakai dapat ditautkan ke satu
// baris log.
func RequestID(c *fiber.Ctx) string {
	id, _ := c.Locals("requestid").(string)
	return id
}

// Fail dan FailValidation SENGAJA DIHAPUS pada pertemuan 7.
//
// Selama keduanya masih ada, akan selalu ada godaan memakainya, dan satu
// pemakaian saja sudah cukup untuk membuat bentuk response tidak lagi
// seragam. Sekarang compiler yang menegakkan aturan ini, bukan ingatan
// penulisnya: handler hanya boleh MENGEMBALIKAN error.
