package helper

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// ================================================================
// JWTManager menerima secret dari LUAR, bukan membacanya sendiri dari
// package config.
//
// Alasannya bukan selera: config sudah mengimpor route, route mengimpor
// service, dan service mengimpor helper. Bila helper ikut mengimpor
// config, terbentuk lingkaran impor dan Go menolak mengompilasi.
// Menerima dependensi lewat parameter memutus lingkaran itu.
// ================================================================

var (
	ErrInvalidToken = errors.New("token tidak valid")
	ErrExpiredToken = errors.New("token sudah kedaluwarsa")
)

// accessClaims adalah isi access token. Selain claim bawaan JWT,
// username dan role ikut dibawa agar middleware tidak perlu menanyakannya
// ke database pada setiap request, itulah keuntungan utama token
// dibanding session.
//
// Konsekuensinya: bila role seorang pemakai diubah, token lama masih
// membawa role yang lama sampai kedaluwarsa. Itu harga yang dibayar,
// dan itulah sebabnya access token dibuat berumur pendek.
type accessClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
}

func NewJWTManager(secret, issuer string, accessTTL time.Duration) *JWTManager {
	return &JWTManager{
		secret:    []byte(secret),
		issuer:    issuer,
		accessTTL: accessTTL,
	}
}

func (m *JWTManager) AccessTTL() time.Duration { return m.accessTTL }

// GenerateAccess membuat access token berumur pendek.
//
// Perhatikan apa yang TIDAK ada di dalam claims: password, email, dan
// data pribadi apa pun. Payload JWT hanya di-encode base64url, bukan
// dienkripsi, siapa pun yang memegang token dapat membacanya tanpa kunci.
func (m *JWTManager) GenerateAccess(u model.User) (string, error) {
	now := time.Now()

	claims := accessClaims{
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.Itoa(u.ID),
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse memeriksa tanda tangan dan masa berlaku token, lalu mengembalikan
// identitas yang dibawanya.
func (m *JWTManager) Parse(tokenString string) (model.AuthUser, error) {
	claims := &accessClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (any, error) {
			// ==================================================
			// PEMERIKSAAN WAJIB, penutup algorithm confusion.
			//
			// Tanpa baris ini, penyerang dapat mengirim token yang
			// header-nya berbunyi {"alg":"none"} tanpa signature sama
			// sekali, dan pustaka JWT yang lengah akan menerimanya
			// sebagai sah. Varian lain: menukar HS256 dengan RS256
			// agar public key diperlakukan sebagai secret.
			//
			// Yang benar adalah menyatakan algoritma yang DIHARAPKAN,
			// bukan mempercayai algoritma yang DIKLAIM token.
			// ==================================================
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("algoritma tidak diharapkan: %v", t.Header["alg"])
			}
			return m.secret, nil
		},
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(), // token tanpa exp ditolak, bukan dianggap abadi
	)

	if err != nil {
		// Kedaluwarsa dibedakan dari tidak valid karena client memang
		// perlu tahu kapan harus memanggil /auth/refresh.
		if errors.Is(err, jwt.ErrTokenExpired) {
			return model.AuthUser{}, ErrExpiredToken
		}
		return model.AuthUser{}, ErrInvalidToken
	}
	if !token.Valid {
		return model.AuthUser{}, ErrInvalidToken
	}

	userID, err := strconv.Atoi(claims.Subject)
	if err != nil {
		return model.AuthUser{}, ErrInvalidToken
	}

	return model.AuthUser{
		UserID:   userID,
		Username: claims.Username,
		Role:     claims.Role,
	}, nil
}
