package service

import (
	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/helper"
)

// CanAccessStudent memutuskan apakah seseorang boleh menyentuh satu baris
// data mahasiswa. Fungsi MURNI: tidak mengimpor fiber maupun repository,
// jadi bisa diuji cukup dengan memanggilnya.
//
// Dua jalur yang diizinkan:
//  1. Kepemilikan, owner_id data itu sama dengan id pemanggil.
//  2. Permission , role-nya punya hak ":any" atas data siapa pun.
//
// ownerID <= 0 berarti data tak bertuan (owner_id NULL). Pemeriksaan
// ownerID > 0 menjaga agar id 0 tidak pernah dianggap "milik" siapa pun,
// sekalipun ada AuthUser kosong yang UserID-nya bernilai nol.
func CanAccessStudent(
	current model.AuthUser,
	ownerID int,
	perms *helper.PermissionSet,
	anyPermission string,
) bool {
	if ownerID > 0 && current.UserID == ownerID {
		return true
	}
	return perms.Can(current.Role, anyPermission)
}
