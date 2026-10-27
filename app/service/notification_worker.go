package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/helper"
)

// NotificationWorker mengerjakan task "notification:send".
//
// Bagi worker, Asynq adalah framework-nya, sama seperti Fiber bagi API.
// Karena itu method di sini menerima *asynq.Task, seperti method service
// lain menerima *fiber.Ctx.
type NotificationWorker struct {
	users         repository.UserRepository
	notifications repository.NotificationRepository
	tx            *repository.TxManager
	mailer        helper.Mailer
	logger        *slog.Logger
}

func NewNotificationWorker(
	users repository.UserRepository,
	notifications repository.NotificationRepository,
	tx *repository.TxManager,
	mailer helper.Mailer,
	logger *slog.Logger,
) *NotificationWorker {
	return &NotificationWorker{users: users, notifications: notifications,
		tx: tx, mailer: mailer, logger: logger}
}

// HandleSend mengembalikan error untuk meminta Asynq melakukan retry nanti,
// atau error yang membungkus asynq.SkipRetry untuk menyatakan "jangan
// di-retry, task ini tidak mungkin berhasil".
//
// Asynq menjamin setiap task dijalankan PALING SEDIKIT satu kali, bukan TEPAT
// satu kali. Karena itu handler ini harus idempoten: menjalankannya dua kali
// untuk task yang sama harus berakibat sama dengan menjalankannya sekali.
func (w *NotificationWorker) HandleSend(ctx context.Context, t *asynq.Task) error {
	var job model.NotificationJob
	if err := json.Unmarshal(t.Payload(), &job); err != nil {
		return fmt.Errorf("%w: payload task rusak: %v", asynq.SkipRetry, err)
	}

	if job.EventType == model.EventCourseQuotaChanged {
		return w.handleQuotaChanged(ctx, job)
	}

	var event model.EnrollmentEvent
	if err := json.Unmarshal(job.Payload, &event); err != nil {
		return fmt.Errorf("%w: payload event %d rusak: %v", asynq.SkipRetry, job.OutboxID, err)
	}

	user, err := w.users.FindByID(ctx, event.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		// Akunnya sudah dihapus. Retry tidak akan memunculkannya.
		return fmt.Errorf("%w: user %d tidak ditemukan", asynq.SkipRetry, event.UserID)
	}
	if err != nil {
		return err // database sedang bermasalah: layak di-retry
	}

	msg, err := BuildMessage(job.EventType, event, user)
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	alreadySent, err := w.claimAndSend(ctx, job.OutboxID, user, msg)
	if errors.Is(err, helper.ErrPermanentDelivery) {
		// SkipRetry DIBUNGKUS dengan %w supaya errors.Is di dalam Asynq
		// menemukannya. Error aslinya cukup ikut sebagai teks.
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	if err != nil {
		return err
	}

	retry, _ := asynq.GetRetryCount(ctx)
	w.logger.Info("notification_processed",
		slog.Int64("outbox_id", job.OutboxID),
		slog.String("event", job.EventType),
		slog.Int("retry", retry),
		slog.Bool("already_sent", alreadySent))
	return nil
}

// claimAndSend menjalankan claim dan pengiriman di dalam SATU transaction:
//   - pengiriman gagal  -> claim ikut dibatalkan -> attempt berikutnya masih
//     bisa mengirim;
//   - pengiriman sukses -> claim di-commit -> attempt berikutnya melihat claim
//     ini dan tidak mengirim lagi.
//
// Nilai pertama bernilai true bila notifikasi untuk pasangan (outbox_id,
// user) sudah pernah terkirim sebelumnya.
func (w *NotificationWorker) claimAndSend(ctx context.Context, outboxID int64, user model.User, msg model.Message) (bool, error) {
	alreadySent := false
	err := w.tx.WithTxOnce(ctx, func(tx repository.DBTX) error {
		// Claim WAJIB lewat repository hasil WithTx. Claim lewat pool
		// langsung tersimpan sendiri dan tidak ikut batal bila Send gagal.
		claimed, err := w.notifications.WithTx(tx).Claim(ctx, model.Notification{
			OutboxID: outboxID, UserID: user.ID, Channel: "email",
			Subject: msg.Subject, Body: msg.Body,
		})
		if err != nil {
			return err
		}
		if !claimed {
			alreadySent = true
			return nil
		}
		return w.mailer.Send(ctx, msg)
	})
	return alreadySent, err
}

// handleQuotaChanged mengirim satu email ke setiap peserta (Tugas Mandiri D.2).
//
// Satu event, satu task, banyak penerima. Setiap penerima punya claim dan
// transaction-nya sendiri dengan kunci (outbox_id, user_id). Bila email ke-7
// gagal sementara, enam claim pertama sudah di-commit, task di-retry, dan
// pada attempt berikutnya enam penerima itu dilewati (already_sent) sehingga
// mereka tidak menerima email kedua. Pengiriman dimulai lagi dari penerima
// ke-7.
func (w *NotificationWorker) handleQuotaChanged(ctx context.Context, job model.NotificationJob) error {
	var event model.QuotaChangedEvent
	if err := json.Unmarshal(job.Payload, &event); err != nil {
		return fmt.Errorf("%w: payload event %d rusak: %v", asynq.SkipRetry, job.OutboxID, err)
	}

	retry, _ := asynq.GetRetryCount(ctx)
	sent, skipped, failed := 0, 0, 0
	for _, userID := range event.Participants {
		user, err := w.users.FindByID(ctx, userID)
		if errors.Is(err, repository.ErrNotFound) {
			// Akun peserta sudah dihapus: lewati, peserta lain tetap dikirimi.
			failed++
			continue
		}
		if err != nil {
			return err
		}

		alreadySent, err := w.claimAndSend(ctx, job.OutboxID, user, BuildQuotaMessage(event, user))
		if errors.Is(err, helper.ErrPermanentDelivery) {
			// Alamat satu peserta tidak ada. Retry tidak akan menolong, dan
			// tidak boleh menahan peserta lain. Dicatat lalu dilewati.
			w.logger.Warn("notification_undeliverable",
				slog.Int64("outbox_id", job.OutboxID),
				slog.Int("user_id", userID),
				slog.String("error", err.Error()))
			failed++
			continue
		}
		if err != nil {
			// Kegagalan sementara: hentikan task ini dan biarkan Asynq
			// melakukan retry. Penerima yang sudah ter-claim aman.
			w.logger.Info("notification_fanout_interrupted",
				slog.Int64("outbox_id", job.OutboxID),
				slog.Int("retry", retry),
				slog.Int("sent", sent),
				slog.Int("already_sent", skipped))
			return err
		}
		if alreadySent {
			skipped++
		} else {
			sent++
		}
	}

	w.logger.Info("notification_processed",
		slog.Int64("outbox_id", job.OutboxID),
		slog.String("event", job.EventType),
		slog.Int("retry", retry),
		slog.Int("recipients", len(event.Participants)),
		slog.Int("sent", sent),
		slog.Int("already_sent", skipped),
		slog.Int("undeliverable", failed))
	return nil
}
