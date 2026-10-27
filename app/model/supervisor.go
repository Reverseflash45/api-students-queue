package model

import "time"

// Supervisor adalah dosen pembimbing tugas akhir.
type Supervisor struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Quota           int       `json:"quota"`
	SupervisedCount int       `json:"supervised_count"`
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
}

// Supervision adalah pasangan mahasiswa dan pembimbingnya.
type Supervision struct {
	StudentID    int       `json:"student_id"`
	SupervisorID int       `json:"supervisor_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type AssignSupervisorRequest struct {
	StudentID int `json:"student_id" validate:"required,min=1"`
}

// SwitchSupervisorRequest tidak membawa pembimbing lama. Pembimbing lama
// dibaca dari database SETELAH baris mahasiswa dikunci, supaya keputusan
// selalu memakai data terbaru.
type SwitchSupervisorRequest struct {
	StudentID      int `json:"student_id"       validate:"required,min=1"`
	ToSupervisorID int `json:"to_supervisor_id" validate:"required,min=1"`
}
