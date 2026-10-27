package model

import "time"

// Portfolio adalah portofolio satu mahasiswa. Data mahasiswanya tetap di
// PostgreSQL (tabel students); yang disimpan di MongoDB hanya portofolionya,
// dihubungkan lewat StudentID.
type Portfolio struct {
	ID           string        `json:"id"`
	StudentID    int           `json:"student_id"`
	Headline     string        `json:"headline"`
	Bio          string        `json:"bio"`
	Projects     []Project     `json:"projects"`
	Certificates []Certificate `json:"certificates"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type Project struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Technologies []string `json:"technologies"`
	URL          string   `json:"url"`
	Year         int      `json:"year"`
}

// Certificate.IssuedOn dikirim dan diterima sebagai "YYYY-MM-DD". Di MongoDB
// disimpan sebagai tipe date supaya bisa diurutkan dan dibandingkan.
type Certificate struct {
	Name     string `json:"name"`
	Issuer   string `json:"issuer"`
	IssuedOn string `json:"issued_on"`
}

type ProjectRequest struct {
	Title        string   `json:"title"        validate:"required,min=3,max=120"`
	Technologies []string `json:"technologies" validate:"required,min=1,max=10,dive,min=1,max=30"`
	URL          string   `json:"url"          validate:"omitempty,url,max=300"`
	Year         int      `json:"year"         validate:"required,min=2000,max=2100"`
}

type CertificateRequest struct {
	Name     string `json:"name"      validate:"required,min=3,max=150"`
	Issuer   string `json:"issuer"    validate:"required,min=2,max=120"`
	IssuedOn string `json:"issued_on" validate:"required,datetime=2006-01-02"`
}

// PutPortfolioRequest mengganti SELURUH isi portofolio (semantik PUT).
// Jumlah proyek dibatasi 20, sama dengan batas pada penambahan satu per satu.
type PutPortfolioRequest struct {
	Headline     string               `json:"headline"     validate:"required,min=3,max=120"`
	Bio          string               `json:"bio"          validate:"max=1000"`
	Projects     []ProjectRequest     `json:"projects"     validate:"max=20,dive"`
	Certificates []CertificateRequest `json:"certificates" validate:"max=20,dive"`
}

// TechnologyCount adalah satu baris statistik teknologi.
type TechnologyCount struct {
	Technology   string `json:"technology"`
	ProjectCount int    `json:"project_count"`
	StudentCount int    `json:"student_count"`
}

// PortfolioBrief dipakai pencarian portofolio berdasarkan teknologi.
type PortfolioBrief struct {
	StudentID int    `json:"student_id"`
	Headline  string `json:"headline"`
}
