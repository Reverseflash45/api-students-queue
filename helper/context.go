package helper

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// LocalsAuthUser adalah kunci penyimpanan identitas pemakai di dalam
// context request. Dibuat sebagai konstanta, bukan string yang diketik
// ulang di dua tempat: satu salah ketik pada tempat membaca akan membuat
// identitas selalu kosong, dan kompilator tidak akan menegur apa pun.
const LocalsAuthUser = "authUser"

// CurrentUser membaca identitas yang sudah dipasang RequireAuth.
//
// Nilai kedua bernilai false bila middleware belum pernah berjalan pada
// route ini, pemanggilnya wajib memeriksa, jangan diabaikan. Itulah
// jaring pengaman bila kelak ada route terlindungi yang lupa dipasangi
// RequireAuth.
func CurrentUser(c *fiber.Ctx) (model.AuthUser, bool) {
	user, ok := c.Locals(LocalsAuthUser).(model.AuthUser)
	return user, ok
}
