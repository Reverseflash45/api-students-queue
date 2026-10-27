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

// PortfolioService menghubungkan dua database: keberadaan dan pemilik
// mahasiswa dibaca dari PostgreSQL (tabel students), portofolionya disimpan
// di MongoDB (collection student_portfolios).
type PortfolioService struct {
	portfolios repository.PortfolioRepository
	students   repository.StudentRepository
	perms      *helper.PermissionSet
}

func NewPortfolioService(
	portfolios repository.PortfolioRepository,
	students repository.StudentRepository,
	perms *helper.PermissionSet,
) *PortfolioService {
	return &PortfolioService{portfolios: portfolios, students: students, perms: perms}
}

// studentForRead memastikan mahasiswanya ada di PostgreSQL. Tidak ada
// FOREIGN KEY lintas database, jadi pemeriksaan ini tugas service.
func (s *PortfolioService) studentForRead(ctx context.Context, c *fiber.Ctx) (int, error) {
	id, valid := helper.ParamID(c)
	if !valid {
		return 0, helper.BadRequest("id mahasiswa harus berupa angka positif")
	}
	if _, err := s.students.FindOwnerID(ctx, id); err != nil {
		return 0, translatePortfolioError(err)
	}
	return id, nil
}

// studentForWrite memakai aturan hak akses dari Modul 6: pemilik data
// mahasiswa (students.owner_id) atau pemegang student:update:any.
func (s *PortfolioService) studentForWrite(ctx context.Context, c *fiber.Ctx) (int, error) {
	current, ok := helper.CurrentUser(c)
	if !ok {
		return 0, helper.Unauthorized("belum terautentikasi")
	}
	id, valid := helper.ParamID(c)
	if !valid {
		return 0, helper.BadRequest("id mahasiswa harus berupa angka positif")
	}
	ownerID, err := s.students.FindOwnerID(ctx, id)
	if err != nil {
		return 0, translatePortfolioError(err)
	}
	if !CanAccessStudent(current, ownerID, s.perms, "student:update:any") {
		return 0, helper.Forbidden("tidak berhak mengubah portofolio mahasiswa lain")
	}
	return id, nil
}

func toProject(req model.ProjectRequest) model.Project {
	return model.Project{
		Title:        strings.TrimSpace(req.Title),
		Technologies: req.Technologies,
		URL:          strings.TrimSpace(req.URL),
		Year:         req.Year,
	}
}

// normalizeProject dijalankan SEBELUM validasi, supaya batas jumlah
// teknologi dihitung setelah duplikat dibuang.
func normalizeProject(p *model.ProjectRequest) {
	p.Title = strings.TrimSpace(p.Title)
	p.URL = strings.TrimSpace(p.URL)
	p.Technologies = NormalizeTags(p.Technologies)
}

// ---------- GET /students/:id/portfolio ----------

func (s *PortfolioService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := s.studentForRead(ctx, c)
	if err != nil {
		return err
	}
	p, err := s.portfolios.FindByStudent(ctx, id)
	if err != nil {
		return translatePortfolioError(err)
	}
	return helper.Success(c, fiber.StatusOK, "portofolio ditemukan", p)
}

// ---------- PUT /students/:id/portfolio ----------

func (s *PortfolioService) Put(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := s.studentForWrite(ctx, c)
	if err != nil {
		return err
	}

	// Body didekode ke struct bertipe tetap, BUKAN map[string]any. Operator
	// seperti {"$ne": ""} yang dikirim di posisi string langsung gagal
	// didekode di sini dan tidak pernah sampai ke MongoDB.
	var req model.PutPortfolioRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid dengan tipe field yang benar")
	}
	req.Headline = strings.TrimSpace(req.Headline)
	req.Bio = strings.TrimSpace(req.Bio)
	for i := range req.Projects {
		normalizeProject(&req.Projects[i])
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	p := model.Portfolio{StudentID: id, Headline: req.Headline, Bio: req.Bio}
	for _, pr := range req.Projects {
		p.Projects = append(p.Projects, toProject(pr))
	}
	for _, cr := range req.Certificates {
		p.Certificates = append(p.Certificates, model.Certificate{
			Name: strings.TrimSpace(cr.Name), Issuer: strings.TrimSpace(cr.Issuer), IssuedOn: cr.IssuedOn,
		})
	}

	saved, created, err := s.portfolios.Upsert(ctx, p)
	if err != nil {
		return translatePortfolioError(err)
	}
	location := "/api/v1/students/" + strconv.Itoa(id) + "/portfolio"
	if created {
		return helper.Created(c, "portofolio berhasil dibuat", saved, location)
	}
	return helper.Success(c, fiber.StatusOK, "portofolio berhasil diganti", saved)
}

// ---------- POST /students/:id/portfolio/projects ----------

func (s *PortfolioService) AddProject(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := s.studentForWrite(ctx, c)
	if err != nil {
		return err
	}

	var req model.ProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid dengan tipe field yang benar")
	}
	normalizeProject(&req)
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	updated, err := s.portfolios.AddProject(ctx, id, toProject(req))
	if err != nil {
		return translatePortfolioError(err)
	}
	return helper.Created(c, "proyek berhasil ditambahkan", updated,
		"/api/v1/students/"+strconv.Itoa(id)+"/portfolio")
}

// ---------- DELETE /students/:id/portfolio/projects/:projectId ----------

func (s *PortfolioService) RemoveProject(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := s.studentForWrite(ctx, c)
	if err != nil {
		return err
	}
	if err := s.portfolios.RemoveProject(ctx, id, c.Params("projectId")); err != nil {
		return translatePortfolioError(err)
	}
	return helper.NoContent(c)
}

// ---------- GET /portfolios?technology=go ----------

func (s *PortfolioService) Search(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	tech := strings.ToLower(strings.TrimSpace(c.Query("technology")))
	if tech == "" {
		return helper.Validation(map[string]string{"technology": "wajib diisi"})
	}
	limit := c.QueryInt("limit", 20)
	if limit < 1 || limit > 50 {
		limit = 20
	}

	list, err := s.portfolios.FindByTechnology(ctx, tech, limit)
	if err != nil {
		return helper.Internal(err)
	}
	return helper.Success(c, fiber.StatusOK, "daftar portofolio berhasil diambil", list)
}

// ---------- GET /portfolios/stats/technologies ----------

func (s *PortfolioService) TopTechnologies(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	stats, err := s.portfolios.TopTechnologies(ctx, 10)
	if err != nil {
		return helper.Internal(err)
	}
	return helper.Success(c, fiber.StatusOK, "10 teknologi paling banyak dipakai", stats)
}

func translatePortfolioError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound("mahasiswa tidak ditemukan")
	case errors.Is(err, repository.ErrPortfolioNotFound):
		return helper.NewError(fiber.StatusNotFound, "PORTFOLIO_NOT_FOUND",
			"mahasiswa ini belum punya portofolio, buat dulu dengan PUT")
	case errors.Is(err, repository.ErrProjectNotFound):
		return helper.NewError(fiber.StatusNotFound, "PROJECT_NOT_FOUND", "proyek tidak ditemukan")
	case errors.Is(err, repository.ErrInvalidID):
		return helper.BadRequest("id proyek tidak valid")
	case errors.Is(err, repository.ErrProjectLimit):
		return helper.NewError(fiber.StatusConflict, "PROJECT_LIMIT",
			"jumlah proyek sudah mencapai batas "+strconv.Itoa(repository.MaxProjectsPerPortfolio))
	default:
		return helper.Internal(err)
	}
}
