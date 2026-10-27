package middleware

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/helper"
)

// RequirePermission menolak request yang role-nya tidak memiliki
// permission tertentu.
//
// Dipasang pada route yang haknya bisa diputuskan TANPA melihat isi data
// (misalnya "boleh melihat daftar seluruh mahasiswa"). Keputusan yang
// bergantung pada pemilik data ada di layer service, karena middleware
// belum tahu :id yang diminta itu milik siapa.
//
// WAJIB dipasang SETELAH RequireAuth.
func RequirePermission(perms *helper.PermissionSet, permission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := helper.CurrentUser(c)
		if !ok {
			// Tidak ada identitas = RequireAuth belum dipasang. Tolak.
			return helper.Unauthorized("belum terautentikasi")
		}

		if !perms.Can(user.Role, permission) {
			return helper.Forbidden("role " + user.Role + " tidak memiliki hak " + permission)
		}

		return c.Next()
	}
}

// RequireRole memeriksa nama role secara langsung.
//
// Disediakan HANYA sebagai pembanding (tidak dipakai di route):
// menambah role baru berarti menyunting setiap route yang menyebut
// nama role lama, lalu compile dan deploy ulang.
func RequireRole(roles ...string) fiber.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		user, ok := helper.CurrentUser(c)
		if !ok {
			return helper.Unauthorized("belum terautentikasi")
		}

		if _, granted := allowed[user.Role]; !granted {
			return helper.Forbidden("role Anda tidak berhak mengakses endpoint ini")
		}

		return c.Next()
	}
}
