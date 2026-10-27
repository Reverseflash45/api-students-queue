package service

import (
	"strings"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/helper"
)

// CanAccessUser memutuskan apakah seseorang boleh menyentuh data akun
// (table users) milik user lain. Untuk users, pemilik data = dirinya
// sendiri, jadi targetID langsung dibandingkan dengan id pemanggil.
func CanAccessUser(
	current model.AuthUser,
	targetID int,
	perms *helper.PermissionSet,
	anyPermission string,
) bool {
	if current.UserID == targetID {
		return true
	}
	return perms.Can(current.Role, anyPermission)
}

// ValidateAssignRole memeriksa permintaan pergantian role.
//
// Aturan terakhir melindungi sistem dari dirinya sendiri: tanpa itu,
// satu-satunya admin bisa menurunkan dirinya menjadi user biasa dan
// sistem kehilangan admin selamanya.
func ValidateAssignRole(
	current model.AuthUser,
	targetID int,
	req model.AssignRoleRequest,
	perms *helper.PermissionSet,
) map[string]string {
	errs := map[string]string{}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		errs["role"] = "wajib diisi"
		return errs
	}

	if !perms.IsKnownRole(role) {
		errs["role"] = "role tidak dikenal, pilih salah satu dari: " +
			strings.Join(perms.KnownRoles(), ", ")
	}

	if current.UserID == targetID {
		errs["role"] = "tidak boleh mengubah role diri sendiri"
	}

	return errs
}
