// redeliver membuat beberapa salinan task untuk satu event outbox, untuk
// menguji idempotency worker (Spesifikasi C.5).
//
//	go run ./tools/redeliver -outbox 123 -copies 5
//
// Salinan sengaja dibuat TANPA TaskID. Relay memakai TaskID "outbox-<id>"
// supaya event yang sama tidak di-enqueue dua kali; alat ini justru ingin
// meniru keadaan ketika task yang sama tetap datang berkali-kali (worker mati
// sebelum sempat melapor selesai, lease habis, dan seterusnya).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hibiken/asynq"

	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/app/service"
	"github.com/Reverseflash45/api-students-queue/config"
	"github.com/Reverseflash45/api-students-queue/database"
)

func main() {
	outboxID := flag.Int64("outbox", 0, "id baris outbox yang akan dikirim ulang")
	copies := flag.Int("copies", 5, "jumlah salinan task")
	flag.Parse()
	if *outboxID <= 0 || *copies <= 0 {
		fmt.Println("pakai: go run ./tools/redeliver -outbox <id> -copies <n>")
		os.Exit(1)
	}

	config.LoadEnv()
	ctx := context.Background()
	pool, err := database.NewPool(ctx)
	if err != nil {
		fmt.Println("gagal terhubung ke PostgreSQL:", err)
		os.Exit(1)
	}
	defer pool.Close()

	event, err := repository.NewOutboxRepository(pool).FindByID(ctx, *outboxID)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	client := asynq.NewClient(config.RedisOpt())
	defer client.Close()

	for i := 1; i <= *copies; i++ {
		task, err := service.NewNotificationTask(event)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		info, err := client.EnqueueContext(ctx, task)
		if err != nil {
			fmt.Printf("salinan %d gagal: %v\n", i, err)
			os.Exit(1)
		}
		fmt.Printf("salinan %d: task id=%s queue=%s\n", i, info.ID, info.Queue)
	}
	fmt.Printf("%d salinan task untuk outbox %d (%s) sudah masuk Redis\n", *copies, event.ID, event.EventType)
}
