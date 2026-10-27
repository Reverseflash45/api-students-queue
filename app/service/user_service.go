package service

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/helper"
)

// UserService melayani /users (Langkah 6-7 Modul 6).
//
// Project ini tidak pernah punya CRUD users (akun dibuat lewat
// /auth/register), jadi yang dibuat hanya yang dibutuhkan untuk
// mengelola hak akses: daftar, detail, hapus, dan ganti role.
type UserService struct {
	repo  repository.UserRepository
	perms *helper.PermissionSet
}

func NewUserService(repo repository.UserRepository, perms *helper.PermissionSet) *UserService {
	return &UserService{repo: repo, perms: perms}
}

// translateUserError: pesan 404 milik users, bukan "mahasiswa tidak ditemukan".
//
// Sama seperti translateError pada student_service: tidak menerima
// fiber.Ctx, dan yang tidak dikenali menjadi Internal, BUKAN nil.
// Mengembalikan nil di sini berarti kegagalan database terbaca sebagai
// keberhasilan, dan client menerima 200 dengan body kosong.
func translateUserError(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return helper.NotFound("user tidak ditemukan")
	}
	if errors.Is(err, repository.ErrDuplicate) {
		return helper.Conflict("username sudah dipakai")
	}
	return helper.Internal(err)
}

// ---------- GET /users ---------- (dijaga middleware user:list)
//
// Sejak pertemuan 7: cursor pagination + content negotiation.
func (s *UserService) List(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	// Format dipilih SEBELUM query dijalankan. Bila client meminta format
	// yang tidak dapat kita hasilkan, tidak ada gunanya membebani
	// database untuk hasil yang akan dibuang.
	format, err := helper.Negotiate(c, helper.FormatJSON, helper.FormatCSV)
	if err != nil {
		return err
	}

	q, err := helper.ParseCursorQuery(c)
	if err != nil {
		return err
	}

	rows, err := s.repo.FindAfterCursor(ctx, q)
	if err != nil {
		return helper.Internal(err)
	}

	// Baris tambahan hasil limit+1 dipotong di sini. Ia hanya penanda
	// bahwa masih ada halaman berikutnya, bukan bagian dari halaman ini.
	hasMore := len(rows) > q.Limit
	if hasMore {
		rows = rows[:q.Limit]
	}

	if format == helper.FormatCSV {
		return helper.WriteUsersCSV(c, rows)
	}

	meta := &model.CursorMeta{Limit: q.Limit, HasMore: hasMore}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		meta.NextCursor = helper.EncodeCursor(last.CreatedAt, last.ID)
	}

	return helper.SuccessCursor(c, "daftar user berhasil diambil", rows, meta)
}

// ---------- GET /users/:id ---------- (kepemilikan diperiksa di sini)
func (s *UserService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	// Periksa hak SEBELUM data diambil (lihat modul: timing attack).
	if !CanAccessUser(current, id, s.perms, "user:read:any") {
		return helper.Forbidden("tidak berhak mengakses data user lain")
	}

	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateUserError(err)
	}

	return helper.Success(c, fiber.StatusOK, "user ditemukan", user)
}

// ---------- PATCH /users/:id/role ---------- (dijaga middleware role:assign)
func (s *UserService) AssignRole(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	var req model.AssignRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	// Bentuk field diperiksa tag (required, oneof), sisanya aturan
	// antar-field yang tidak dapat ditulis sebagai tag.
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	// 422, bukan 403: haknya cukup, tapi permintaannya melanggar aturan.
	if errs := ValidateAssignRole(current, id, req, s.perms); len(errs) > 0 {
		return helper.Validation(errs)
	}

	result, err := s.repo.UpdateRole(ctx, id, strings.TrimSpace(req.Role))
	if err != nil {
		return translateUserError(err)
	}

	return helper.Success(c, fiber.StatusOK, "role user berhasil diubah", result)
}

// ---------- DELETE /users/:id ---------- (dijaga middleware user:delete)
func (s *UserService) Delete(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	// Punya user:delete tidak berarti boleh menghapus diri sendiri.
	// Middleware tidak tahu :id ini ternyata id pemanggilnya.
	if current.UserID == id {
		return helper.Forbidden("tidak boleh menghapus akun sendiri")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return translateUserError(err)
	}

	return helper.NoContent(c)
}
