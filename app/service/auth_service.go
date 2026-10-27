package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/helper"
)

// refreshTokenBytes = 32 byte acak = 256 bit.
// Ditulis dalam byte, bukan karakter, karena itulah satuan yang menentukan
// entropi. Hasil hex-nya nanti 64 karakter.
const refreshTokenBytes = 32

// pesanLoginGagal sengaja disimpan sebagai konstanta dan dipakai di
// SELURUH jalur kegagalan login.
//
// Bila pesannya ditulis ulang di tiap tempat, cepat atau lambat salah satu
// akan berbeda, "user tidak ditemukan" di satu cabang dan "password
// salah" di cabang lain. Perbedaan itu memberi tahu penyerang username
// mana yang terdaftar (user enumeration), dan daftar username yang sah
// adalah setengah dari pekerjaan menebak akun.
const pesanLoginGagal = "username atau password salah"

type AuthService struct {
	users      repository.UserRepository
	tokens     repository.TokenRepository
	jwt        *helper.JWTManager
	perms      *helper.PermissionSet
	refreshTTL time.Duration
}

func NewAuthService(
	users repository.UserRepository,
	tokens repository.TokenRepository,
	jwtManager *helper.JWTManager,
	perms *helper.PermissionSet,
	refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		users:      users,
		tokens:     tokens,
		jwt:        jwtManager,
		perms:      perms,
		refreshTTL: refreshTTL,
	}
}

// ================================================================
// POST /api/v1/auth/register
// ================================================================

func (s *AuthService) Register(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	// Aturan bentuk kini berupa tag pada RegisterRequest; ValidateRegister
	// dan seluruh isi auth_rules.go sudah dihapus.
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	// Password DI-HASH sebelum menyentuh basis data. Nilai aslinya tidak
	// pernah disimpan, tidak pernah masuk log, dan tidak pernah dikirim
	// balik ke client.
	hashed, err := helper.HashPassword(req.Password)
	if err != nil {
		return helper.Internal(err)
	}

	// Role SELALU ditentukan server, tidak pernah diambil dari request.
	// Bahkan bila client mengirim "role":"admin", nilainya tidak punya
	// tempat untuk masuk karena RegisterRequest memang tidak memiliki
	// field itu, lihat komentar pada app/model/auth.go.
	created, err := s.users.Create(ctx, model.User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashed,
		Role:     "user",
		IsActive: true,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			// Pada endpoint register, 409 memang membocorkan bahwa
			// username itu sudah dipakai, dan itu tidak terhindarkan:
			// pemakai harus tahu mengapa pendaftarannya ditolak.
			// Karena itu perlindungan terhadap penyisiran username
			// bertumpu pada rate limiter, bukan pada penyamaran pesan.
			return helper.Conflict("username atau email sudah dipakai")
		}
		return helper.Internal(err)
	}

	// created memuat field Password berisi hash, tetapi tag json:"-"
	// pada struct User membuatnya tidak ikut terkirim.
	return helper.Created(c, "pendaftaran berhasil", created,
		"/api/v1/auth/me?id="+strconv.Itoa(created.ID))
}

// ================================================================
// POST /api/v1/auth/login
// ================================================================

func (s *AuthService) Login(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	user, err := s.users.FindByUsername(ctx, strings.TrimSpace(req.Username))
	if err != nil {
		// Username tidak ditemukan. Pemeriksaan hash palsu tetap
		// dijalankan agar waktu tanggapnya mirip dengan jalur "password
		// salah" di bawah. Tanpa ini, pesan yang sama persis pun tetap
		// membocorkan jawabannya lewat selisih waktu: jalur ini akan
		// selesai dalam mikrodetik, jalur di bawah dalam ratusan
		// milidetik karena menjalankan bcrypt cost 12.
		helper.VerifyDummyPassword(req.Password)
		return helper.Unauthorized(pesanLoginGagal)
	}

	if !helper.VerifyPassword(user.Password, req.Password) {
		return helper.Unauthorized(pesanLoginGagal)
	}

	// Akun nonaktif dibedakan dengan 403, bukan 401. Identitasnya sudah
	// terbukti benar (authentication berhasil); yang gagal adalah izin
	// memakainya (authorization).
	if !user.IsActive {
		return helper.Forbidden("akun dinonaktifkan")
	}

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return helper.Internal(err)
	}

	return helper.Success(c, fiber.StatusOK, "login berhasil", pair)
}

// ================================================================
// POST /api/v1/auth/refresh
// ================================================================

func (s *AuthService) Refresh(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	if strings.TrimSpace(req.RefreshToken) == "" {
		return helper.BadRequest("refresh_token wajib diisi")
	}

	// Yang dicari di basis data adalah HASH-nya. Token aslinya tidak
	// pernah tersimpan di mana pun, sehingga bocornya isi table
	// refresh_tokens tidak memberi penyerang token yang bisa dipakai.
	hash := helper.SHA256Hex(req.RefreshToken)

	stored, err := s.tokens.FindActive(ctx, hash)
	if err != nil {
		// Satu pesan untuk tiga kemungkinan: token tidak pernah ada,
		// sudah dicabut, atau sudah kedaluwarsa. Membedakannya akan
		// memberi tahu penyerang bahwa tebakan tokennya "hampir benar".
		return helper.Unauthorized("refresh token tidak valid atau sudah kedaluwarsa")
	}

	user, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil || !user.IsActive {
		return helper.Unauthorized("akun tidak dapat dipakai")
	}

	// ROTASI: token lama langsung dicabut dan diganti yang baru, sehingga
	// satu refresh token hanya berguna sekali. Bila token yang sudah
	// dipakai muncul lagi, itu tanda kuat ada dua pihak memegangnya -
	// sistem sungguhan menanggapinya dengan RevokeAllForUser.
	if err := s.tokens.Revoke(ctx, hash); err != nil {
		return helper.Internal(err)
	}

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return helper.Internal(err)
	}

	return helper.Success(c, fiber.StatusOK, "token berhasil diperbarui", pair)
}

// ================================================================
// POST /api/v1/auth/logout
// ================================================================

func (s *AuthService) Logout(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	if strings.TrimSpace(req.RefreshToken) != "" {
		// Kegagalan mencabut sengaja tidak dilaporkan sebagai error.
		// Dari sudut pandang pemakai, logout harus selalu berhasil -
		// jawaban 500 hanya membuat orang mengira dirinya masih masuk.
		//
		// Logout juga menjawab 200 untuk token yang tidak dikenal, agar
		// endpoint ini tidak berubah menjadi alat menebak token yang sah.
		_ = s.tokens.Revoke(ctx, helper.SHA256Hex(req.RefreshToken))
	}

	// CATATAN JUJUR: access token yang sudah terlanjur diterbitkan TETAP
	// sah sampai kedaluwarsa, karena server tidak menyimpan apa pun
	// tentangnya. Logout hanya menghentikan penerbitan token baru.
	// Itulah harga yang dibayar untuk kemudahan token tanpa session,
	// dan itulah sebabnya access token dibuat berumur 15 menit.
	return helper.Success(c, fiber.StatusOK, "logout berhasil", nil)
}

// ================================================================
// GET /api/v1/auth/me
// ================================================================

func (s *AuthService) Me(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	authUser, ok := helper.CurrentUser(c)
	if !ok {
		// Seharusnya tidak pernah terjadi karena route ini dipasangi
		// RequireAuth. Pemeriksaan ini adalah jaring pengaman bila kelak
		// seseorang memindahkan route ini ke grup yang tidak terlindungi.
		return helper.Unauthorized("belum terautentikasi")
	}

	// Data diambil ulang dari basis data, tidak dibaca dari token.
	// Token hanya dipercaya untuk menjawab "siapa Anda", bukan untuk
	// menyimpan data yang bisa berubah: token berumur 15 menit dan
	// isinya membeku sejak diterbitkan.
	user, err := s.users.FindByID(ctx, authUser.UserID)
	if err != nil {
		return helper.Unauthorized("user tidak ditemukan")
	}

	// permissions hanya KEMUDAHAN TAMPILAN untuk frontend (tombol mana
	// yang layak ditampilkan), BUKAN pengamanan. Pengamanan tetap ada di
	// middleware dan service.
	//
	// Permission dihitung dari role di DATABASE (user.Role), bukan dari
	// token, supaya frontend langsung melihat role terbaru.
	return helper.Success(c, fiber.StatusOK, "profil berhasil diambil", fiber.Map{
		"user":        user,
		"permissions": s.perms.PermissionsOf(user.Role),
	})
}

// ================================================================
// issueTokenPair
// ================================================================

// issueTokenPair membuat access token dan refresh token sekaligus.
//
// Dipakai oleh Login dan Refresh. Dikumpulkan di satu function agar
// keduanya tidak mungkin menghasilkan bentuk token yang berbeda.
func (s *AuthService) issueTokenPair(
	ctx context.Context, user model.User,
) (model.TokenPair, error) {
	accessToken, err := s.jwt.GenerateAccess(user)
	if err != nil {
		return model.TokenPair{}, err
	}

	refreshToken, err := helper.RandomToken(refreshTokenBytes)
	if err != nil {
		return model.TokenPair{}, err
	}

	// Yang disimpan hash-nya; nilai aslinya hanya dikirim ke client
	// sekali ini saja dan tidak dapat diperoleh kembali dari server.
	err = s.tokens.Save(ctx, model.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.SHA256Hex(refreshToken),
		ExpiresAt: time.Now().Add(s.refreshTTL),
	})
	if err != nil {
		return model.TokenPair{}, err
	}

	return model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwt.AccessTTL().Seconds()),
	}, nil
}
