package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/app/service"
	"github.com/Reverseflash45/api-students-queue/config"
	"github.com/Reverseflash45/api-students-queue/database"
	"github.com/Reverseflash45/api-students-queue/helper"
	"github.com/Reverseflash45/api-students-queue/route"
)

// minSecretLength = 32 karakter. Secret HS256 yang pendek bisa ditebak dari
// satu token yang pernah terlihat, dan siapa pun yang memegangnya bisa
// mencetak token atas nama siapa saja.
const minSecretLength = 32

func main() {
	config.LoadEnv()

	// Satu program, tiga cara menjalankan:
	//   go run . -role=api     hanya melayani HTTP
	//   go run . -role=worker  hanya relay outbox dan worker queue
	//   go run . -role=all     keduanya dalam satu proses (bawaan)
	// Di production, api dan worker dijalankan sebagai proses terpisah supaya
	// masing-masing bisa diperbanyak sesuai bebannya.
	role := flag.String("role", config.GetEnv("APP_ROLE", "all"), "api | worker | all")
	flag.Parse()

	// Setiap role menulis ke berkas log sendiri: logs/api.log,
	// logs/worker.log, atau logs/all.log.
	logger := config.NewLoggerFile(*role + ".log")

	runAPI := *role == "api" || *role == "all"
	runWorker := *role == "worker" || *role == "all"
	if !runAPI && !runWorker {
		logger.Error("role tidak dikenal", slog.String("role", *role))
		os.Exit(1)
	}

	pool, err := database.NewPool(context.Background())
	if err != nil {
		logger.Error("gagal terhubung ke database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	// ctx dibatalkan ketika Ctrl+C ditekan atau sistem mengirim SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var app *fiber.App
	if runAPI {
		app = startAPI(logger, pool)
		if app == nil {
			os.Exit(1)
		}
	}

	var stopWorker func()
	if runWorker {
		stopWorker = startWorker(ctx, logger, pool)
		if stopWorker == nil {
			os.Exit(1)
		}
	}

	logger.Info("aplikasi berjalan", slog.String("role", *role))
	<-ctx.Done()
	logger.Info("sinyal berhenti diterima", slog.String("role", *role))

	// Urutan berhenti: pintu masuk ditutup lebih dulu (HTTP), baru kemudian
	// pekerja di belakangnya.
	if app != nil {
		if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
			logger.Error("shutdown HTTP tidak rapi", slog.String("error", err.Error()))
		}
	}
	if stopWorker != nil {
		stopWorker()
	}
	logger.Info("aplikasi berhenti dengan rapi")
}

// startAPI menyiapkan seluruh kebutuhan HTTP. Mengembalikan nil bila gagal.
// Isinya perakitan dari main.go pertemuan 10, ditambah outbox dan
// NotificationService.
func startAPI(logger *slog.Logger, pool *pgxpool.Pool) *fiber.App {
	jwtSecret := config.GetEnv("JWT_SECRET", "")
	if len(jwtSecret) < minSecretLength {
		logger.Error("JWT_SECRET tidak diisi atau terlalu pendek",
			slog.Int("minimal_karakter", minSecretLength))
		return nil
	}

	mongoClient, err := database.NewMongoClient(context.Background())
	if err != nil {
		logger.Error("gagal terhubung ke MongoDB", slog.String("error", err.Error()))
		return nil
	}
	mongoDB := mongoClient.Database(config.GetEnv("MONGO_DATABASE", "praktikum_backend"))

	studentRepository := repository.NewStudentRepository(pool)
	userRepository := repository.NewUserRepository(pool)
	tokenRepository := repository.NewTokenRepository(pool)
	roleRepository := repository.NewRoleRepository(pool)
	courseRepository := repository.NewCourseRepository(pool)
	supervisorRepository := repository.NewSupervisorRepository(pool)
	outboxRepository := repository.NewOutboxRepository(pool)
	notificationRepository := repository.NewNotificationRepository(pool)
	txManager := repository.NewTxManager(pool)
	reviewRepository := repository.NewReviewRepository(mongoDB)
	portfolioRepository := repository.NewPortfolioRepository(mongoDB)

	if err := reviewRepository.EnsureIndexes(context.Background()); err != nil {
		logger.Error("gagal membuat index MongoDB", slog.String("error", err.Error()))
		return nil
	}
	if err := portfolioRepository.EnsureIndexes(context.Background()); err != nil {
		logger.Error("gagal membuat index MongoDB", slog.String("error", err.Error()))
		return nil
	}

	// Pemetaan role ke permission dibaca SEKALI saat aplikasi menyala.
	rawPermissions, err := roleRepository.LoadPermissions(context.Background())
	if err != nil {
		logger.Error("gagal memuat permission", slog.String("error", err.Error()))
		return nil
	}
	permissions := helper.NewPermissionSet(rawPermissions)
	logger.Info("permission dimuat", slog.Any("roles", permissions.KnownRoles()))

	jwtManager := helper.NewJWTManager(
		jwtSecret,
		config.GetEnv("JWT_ISSUER", "praktikum-backend"),
		time.Duration(config.GetEnvInt("JWT_ACCESS_TTL_MINUTES", 15))*time.Minute,
	)

	// Inspector hanya MEMBACA keadaan queue untuk endpoint admin. Ia tidak
	// menghubungi Redis saat dibuat, jadi API tetap bisa menyala walaupun
	// Redis sedang mati.
	inspector := asynq.NewInspector(config.RedisOpt())

	app := config.NewApp(logger, route.Dependencies{
		Pool:           pool,
		Mongo:          mongoClient,
		JWT:            jwtManager,
		Permissions:    permissions,
		StudentService: service.NewStudentService(studentRepository, permissions),
		UserService:    service.NewUserService(userRepository, permissions),
		AuthService: service.NewAuthService(
			userRepository, tokenRepository, jwtManager, permissions,
			time.Duration(config.GetEnvInt("JWT_REFRESH_TTL_DAYS", 7))*24*time.Hour,
		),

		CourseService:     service.NewCourseService(courseRepository, outboxRepository, txManager),
		SupervisorService: service.NewSupervisorService(supervisorRepository, txManager),
		ReviewService:     service.NewReviewService(reviewRepository, courseRepository, permissions),
		PortfolioService:  service.NewPortfolioService(portfolioRepository, studentRepository, permissions),

		NotificationService: service.NewNotificationService(
			notificationRepository, outboxRepository, inspector),
	})

	// Sumber daya milik API ditutup setelah server HTTP berhenti.
	app.Hooks().OnShutdown(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = inspector.Close()
		return mongoClient.Disconnect(ctx)
	})

	port := config.GetEnv("APP_PORT", "3000")
	go func() {
		if err := app.Listen(":" + port); err != nil {
			logger.Error("server berhenti", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()
	logger.Info("HTTP berjalan", slog.String("port", port))
	return app
}
