package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/helper"
)

// ================================================================
// StudentService, sejak pertemuan 7, seluruh method MENGEMBALIKAN
// error, bukan menuliskan response kegagalan sendiri. Satu-satunya
// tempat error berubah menjadi response HTTP adalah ErrorHandler pada
// config/app.go.
// ================================================================

type StudentService struct {
	repo  repository.StudentRepository
	perms *helper.PermissionSet
}

func NewStudentService(
	repo repository.StudentRepository,
	perms *helper.PermissionSet,
) *StudentService {
	return &StudentService{repo: repo, perms: perms}
}

// translateError mengubah error milik repository menjadi AppError.
//
// Perhatikan tanda tangannya: tidak ada fiber.Ctx. Fungsi ini hanya
// menerjemahkan satu jenis error menjadi jenis lain, dan tidak tahu
// apa pun tentang HTTP. Yang tidak dikenali menjadi Internal, fail
// closed: lebih baik membalas 500 daripada menebak-nebak status, dan
// JAUH lebih baik daripada mengembalikan nil, yang membuat kegagalan
// terbaca sebagai keberhasilan.
func translateError(err error, entity string) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound(entity + " tidak ditemukan")
	case errors.Is(err, repository.ErrDuplicate):
		return helper.Conflict("NIM sudah dipakai mahasiswa lain")
	default:
		return helper.Internal(err)
	}
}

// authorizeStudent adalah pemeriksaan kepemilikan dari Modul 6, kini
// mengembalikan error alih-alih menulis response.
func (s *StudentService) authorizeStudent(
	ctx context.Context, c *fiber.Ctx, id int, anyPermission string,
) error {
	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	if s.perms.Can(current.Role, anyPermission) {
		return nil
	}

	ownerID, err := s.repo.FindOwnerID(ctx, id)
	if err != nil {
		return translateError(err, "mahasiswa")
	}

	if !CanAccessStudent(current, ownerID, s.perms, anyPermission) {
		return helper.Forbidden("tidak berhak mengakses data mahasiswa milik user lain")
	}

	return nil
}

// ================================================================
// GET /api/v1/students, cursor pagination + content negotiation
// ================================================================

func (s *StudentService) List(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	// Format dipilih SEBELUM query dijalankan. Bila client meminta format
	// yang tidak dapat kita hasilkan, tidak ada gunanya membebani database
	// untuk hasil yang akan dibuang.
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

	// Baris tambahan hasil limit+1 dipotong di sini. Ia hanya penanda bahwa
	// masih ada halaman berikutnya, bukan bagian dari halaman ini.
	hasMore := len(rows) > q.Limit
	if hasMore {
		rows = rows[:q.Limit]
	}

	if format == helper.FormatCSV {
		return helper.WriteStudentsCSV(c, rows)
	}

	meta := &model.CursorMeta{Limit: q.Limit, HasMore: hasMore}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		meta.NextCursor = helper.EncodeCursor(last.CreatedAt, last.ID)
	}

	return helper.SuccessCursor(c, "daftar mahasiswa berhasil diambil", rows, meta)
}

// ================================================================
// GET /api/v1/students/:id
// ================================================================

func (s *StudentService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	if err := s.authorizeStudent(ctx, c, id, "student:read:any"); err != nil {
		return err
	}

	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateError(err, "mahasiswa")
	}

	return helper.Success(c, fiber.StatusOK, "mahasiswa ditemukan", student)
}

// ================================================================
// POST /api/v1/students
// ================================================================

func (s *StudentService) Create(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	var req model.CreateStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	req.NIM = strings.TrimSpace(req.NIM)
	req.Name = strings.TrimSpace(req.Name)

	// Validasi deklaratif: aturannya ada pada tag struct, bukan pada
	// rangkaian if di berkas ini.
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	baru, err := s.repo.Create(ctx, model.Student{
		NIM:      req.NIM,
		Name:     req.Name,
		Grade:    req.Grade,
		IsActive: true,
		OwnerID:  &current.UserID, // selalu dari token, tidak pernah dari body
	})
	if err != nil {
		return translateError(err, "mahasiswa")
	}

	return helper.Created(c, "mahasiswa berhasil ditambahkan", baru,
		"/api/v1/students/"+strconv.Itoa(baru.ID))
}

// ================================================================
// PUT /api/v1/students/:id
// ================================================================

func (s *StudentService) Replace(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	if err := s.authorizeStudent(ctx, c, id, "student:update:any"); err != nil {
		return err
	}

	var req model.ReplaceStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	req.NIM = strings.TrimSpace(req.NIM)
	req.Name = strings.TrimSpace(req.Name)

	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	hasil, err := s.repo.Update(ctx, model.Student{
		ID:       id,
		NIM:      req.NIM,
		Name:     req.Name,
		Grade:    req.Grade,
		IsActive: req.IsActive,
	})
	if err != nil {
		return translateError(err, "mahasiswa")
	}

	return helper.Success(c, fiber.StatusOK, "mahasiswa berhasil diganti seluruhnya", hasil)
}

// ================================================================
// PATCH /api/v1/students/:id
// ================================================================

func (s *StudentService) Patch(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	if err := s.authorizeStudent(ctx, c, id, "student:update:any"); err != nil {
		return err
	}

	var req model.PatchStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	// Body tanpa satu pun field adalah permintaan yang tidak masuk akal,
	// bukan pelanggaran aturan field: 400, bukan 422. Aturan ini tidak
	// dapat ditulis sebagai tag karena ia berbicara tentang HUBUNGAN
	// antar field.
	if IsEmptyPatch(req) {
		return helper.BadRequest("tidak ada field yang diubah")
	}

	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	saatIni, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateError(err, "mahasiswa")
	}

	hasil, err := s.repo.Update(ctx, ApplyPatch(saatIni, req))
	if err != nil {
		return translateError(err, "mahasiswa")
	}

	return helper.Success(c, fiber.StatusOK, "mahasiswa berhasil diperbarui sebagian", hasil)
}

// ================================================================
// DELETE /api/v1/students/:id
// ================================================================

func (s *StudentService) Delete(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return translateError(err, "mahasiswa")
	}

	return helper.NoContent(c)
}
