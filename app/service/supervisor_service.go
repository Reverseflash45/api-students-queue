package service

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/helper"
)

var errSameSupervisor = errors.New("pembimbing baru sama dengan pembimbing lama")

// SupervisorService mengurus penetapan dosen pembimbing tugas akhir.
//
// Urutan kunci di seluruh method sama: baris students lebih dulu, baru
// baris supervisors dengan id menaik. Selama semua transaction mengikuti
// urutan ini, lingkaran tunggu (deadlock) tidak bisa terbentuk.
type SupervisorService struct {
	supervisors repository.SupervisorRepository
	tx          *repository.TxManager
}

func NewSupervisorService(supervisors repository.SupervisorRepository, tx *repository.TxManager) *SupervisorService {
	return &SupervisorService{supervisors: supervisors, tx: tx}
}

// ---------- GET /supervisors ----------

func (s *SupervisorService) List(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	list, err := s.supervisors.FindAll(ctx)
	if err != nil {
		return helper.Internal(err)
	}
	return helper.Success(c, fiber.StatusOK, "daftar pembimbing berhasil diambil", list)
}

// ---------- GET /supervisors/:id ----------

func (s *SupervisorService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}
	sup, err := s.supervisors.FindByID(ctx, id)
	if err != nil {
		return translateSupervisorError(err)
	}
	return helper.Success(c, fiber.StatusOK, "pembimbing ditemukan", sup)
}

// ---------- POST /supervisors/:id/assign ----------

func (s *SupervisorService) Assign(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	supervisorID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	var req model.AssignSupervisorRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	var sup model.Supervisor

	err := s.tx.WithTx(ctx, func(tx repository.DBTX) error {
		repo := s.supervisors.WithTx(tx)

		// 1. Kunci baris mahasiswa. Dua admin yang menetapkan mahasiswa
		//    yang sama ke dua pembimbing berbeda pada detik yang sama
		//    akan antre di sini, bukan sama-sama jalan.
		if err := repo.LockStudent(ctx, req.StudentID); err != nil {
			return err
		}

		// 2. Ambil satu slot secara atomik (UPDATE bersyarat).
		var err error
		sup, err = repo.IncrementSupervised(ctx, supervisorID)
		if err != nil {
			return err
		}

		// 3. Catat pasangannya. Bila mahasiswa sudah punya pembimbing,
		//    PRIMARY KEY menolak dan rollback ikut mengembalikan slot
		//    yang diambil pada langkah 2.
		return repo.InsertSupervision(ctx, req.StudentID, supervisorID)
	})
	if err != nil {
		return translateSupervisorError(err)
	}

	return helper.Created(c, "pembimbing berhasil ditetapkan", fiber.Map{
		"student_id": req.StudentID,
		"supervisor": sup,
	}, "/api/v1/supervisors/"+strconv.Itoa(supervisorID))
}

// ---------- POST /supervisions/switch ----------

func (s *SupervisorService) Switch(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.SwitchSupervisorRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	var fromID int
	var joined model.Supervisor

	err := s.tx.WithTx(ctx, func(tx repository.DBTX) error {
		repo := s.supervisors.WithTx(tx)

		// 1. Kunci baris mahasiswa dulu. Setelah ini tidak ada transaction
		//    lain yang bisa mengganti pembimbing mahasiswa ini, jadi
		//    pembimbing lama yang dibaca di langkah 2 pasti masih berlaku
		//    sampai transaction selesai.
		if err := repo.LockStudent(ctx, req.StudentID); err != nil {
			return err
		}

		// 2. Baca pembimbing saat ini.
		var err error
		fromID, err = repo.FindSupervisorOf(ctx, req.StudentID)
		if err != nil {
			return err
		}
		if fromID == req.ToSupervisorID {
			return errSameSupervisor
		}

		// 3. Kunci kedua pembimbing dengan urutan id menaik.
		if err := repo.LockSupervisors(ctx, fromID, req.ToSupervisorID); err != nil {
			return err
		}

		// 4. Lepas slot lama, ambil slot baru, pindahkan pasangannya.
		//    Bila pembimbing baru penuh, error di langkah ini membatalkan
		//    juga pelepasan slot lama: mahasiswa tetap dengan pembimbing lama.
		if _, err := repo.DecrementSupervised(ctx, fromID); err != nil {
			return err
		}
		joined, err = repo.IncrementSupervised(ctx, req.ToSupervisorID)
		if err != nil {
			return err
		}
		return repo.MoveSupervision(ctx, req.StudentID, req.ToSupervisorID)
	})
	if err != nil {
		return translateSupervisorError(err)
	}

	return helper.Success(c, fiber.StatusOK, "pembimbing berhasil diganti", fiber.Map{
		"student_id":         req.StudentID,
		"from_supervisor_id": fromID,
		"supervisor":         joined,
	})
}

// ---------- PATCH /supervisors/:id ----------

func (s *SupervisorService) UpdateQuota(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	var req model.UpdateQuotaRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	updated, err := s.supervisors.UpdateQuota(ctx, id, req.Quota, req.Version)
	if err != nil {
		return translateSupervisorError(err)
	}
	return helper.Success(c, fiber.StatusOK, "kuota pembimbing berhasil diubah", updated)
}

func translateSupervisorError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound("pembimbing tidak ditemukan")
	case errors.Is(err, repository.ErrStudentNotFound):
		return helper.NotFound("mahasiswa tidak ditemukan")
	case errors.Is(err, repository.ErrSupervisorFull):
		return helper.NewError(fiber.StatusConflict, "SUPERVISOR_FULL", "kuota pembimbing sudah penuh")
	case errors.Is(err, repository.ErrAlreadySupervised):
		return helper.NewError(fiber.StatusConflict, "ALREADY_SUPERVISED", "mahasiswa sudah memiliki pembimbing")
	case errors.Is(err, repository.ErrNotSupervised):
		return helper.NewError(fiber.StatusConflict, "NOT_SUPERVISED", "mahasiswa belum memiliki pembimbing")
	case errors.Is(err, errSameSupervisor):
		return helper.NewError(fiber.StatusConflict, "SAME_SUPERVISOR", "pembimbing baru sama dengan pembimbing saat ini")
	case errors.Is(err, repository.ErrVersionConflict):
		return helper.NewError(fiber.StatusConflict, "VERSION_CONFLICT",
			"data pembimbing sudah diubah pihak lain, muat ulang lalu coba lagi")
	case errors.Is(err, repository.ErrQuotaBelowSupervised):
		return helper.NewError(fiber.StatusConflict, "QUOTA_BELOW_SUPERVISED",
			"kuota tidak boleh lebih kecil dari jumlah mahasiswa bimbingan saat ini")
	default:
		return helper.Internal(err)
	}
}
