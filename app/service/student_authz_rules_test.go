package service

import (
	"testing"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/helper"
)

func permsStudent() *helper.PermissionSet {
	// Sama persis dengan isi migration 004.
	return helper.NewPermissionSet(map[string][]string{
		"admin": {"student:list", "student:read:any", "student:create",
			"student:update:any", "student:delete"},
		"staff": {"student:list", "student:read:any", "student:create"},
		"user":  {},
	})
}

func TestCanAccessStudent(t *testing.T) {
	perms := permsStudent()

	admin := model.AuthUser{UserID: 1, Role: "admin"}
	staff := model.AuthUser{UserID: 2, Role: "staff"}
	budi := model.AuthUser{UserID: 3, Role: "user"}

	cases := []struct {
		nama    string
		current model.AuthUser
		owner   int
		perm    string
		mau     bool
	}{
		{"user pemilik boleh baca", budi, 3, "student:read:any", true},
		{"user pemilik boleh ubah", budi, 3, "student:update:any", true},
		{"user bukan pemilik tidak boleh baca", budi, 2, "student:read:any", false},
		{"user bukan pemilik tidak boleh ubah", budi, 2, "student:update:any", false},
		{"staff baca data orang lain (punya :any)", staff, 3, "student:read:any", true},
		{"staff ubah data orang lain (tak punya :any)", staff, 3, "student:update:any", false},
		{"staff ubah datanya sendiri", staff, 2, "student:update:any", true},
		{"admin ubah data orang lain", admin, 3, "student:update:any", true},
		{"data tak bertuan: user ditolak", budi, 0, "student:read:any", false},
		{"data tak bertuan: admin boleh", admin, 0, "student:update:any", true},
		{"AuthUser kosong vs data tak bertuan", model.AuthUser{}, 0, "student:read:any", false},
		{"role tak dikenal ditolak", model.AuthUser{UserID: 9, Role: "hacker"}, 3, "student:read:any", false},
	}

	for _, c := range cases {
		t.Run(c.nama, func(t *testing.T) {
			if got := CanAccessStudent(c.current, c.owner, perms, c.perm); got != c.mau {
				t.Fatalf("dapat %v, mau %v", got, c.mau)
			}
		})
	}
}

// PermissionSet nil (lupa diinisialisasi) -> hanya pemilik yang lolos.
func TestCanAccessStudentPermsNil(t *testing.T) {
	admin := model.AuthUser{UserID: 1, Role: "admin"}

	if CanAccessStudent(admin, 3, nil, "student:read:any") {
		t.Fatal("perms nil tidak boleh memberi akses :any")
	}
	if !CanAccessStudent(admin, 1, nil, "student:read:any") {
		t.Fatal("pemilik tetap boleh mengakses datanya sendiri")
	}
}
