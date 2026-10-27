package route

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/Reverseflash45/api-students-queue/app/service"
	"github.com/Reverseflash45/api-students-queue/helper"
	"github.com/Reverseflash45/api-students-queue/middleware"
)

// Dependencies mengumpulkan seluruh dependensi route menjadi satu struct.
//
// Sampai pertemuan 4, Register menerimanya sebagai parameter berderet.
// Dengan bertambahnya service, daftar itu memanjang dan setiap penambahan
// berikutnya memaksa tanda tangan function berubah, beserta semua tempat
// yang memanggilnya. Struct membuat penambahan cukup berupa satu field.
type Dependencies struct {
	Pool           *pgxpool.Pool
	Mongo          *mongo.Client
	JWT            *helper.JWTManager
	Permissions    *helper.PermissionSet
	StudentService *service.StudentService
	UserService    *service.UserService
	AuthService    *service.AuthService

	CourseService     *service.CourseService
	SupervisorService *service.SupervisorService
	ReviewService     *service.ReviewService
	PortfolioService  *service.PortfolioService

	NotificationService *service.NotificationService
}

// Register memetakan URL ke method pada service.
//
// Pembagiannya sengaja dibuat dapat dibaca sekilas: mana yang publik,
// mana yang untuk masuk, dan mana yang terkunci. Bila kelak seseorang
// bertanya "endpoint apa saja yang bisa diakses tanpa login", jawabannya
// cukup ditunjukkan dari berkas ini, tidak perlu menyisir seluruh
// service satu per satu.
func Register(app *fiber.App, deps Dependencies) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	api := app.Group("/api/v1")

	// ============================================================
	// PUBLIK, tidak memerlukan token
	// ============================================================
	//
	// /health sengaja dibiarkan terbuka. Endpoint kesehatan dipanggil
	// oleh load balancer dan alat pemantauan yang tidak memiliki akun,
	// dan isinya memang tidak membocorkan apa pun.
	api.Get("/health", healthCheck(deps.Pool, deps.Mongo))

	// ============================================================
	// AUTENTIKASI, pintu masuk
	// ============================================================
	auth := api.Group("/auth", middleware.RequireJSON)

	auth.Post("/register", deps.AuthService.Register)

	// Rate limiter dipasang HANYA pada login, bukan pada seluruh grup.
	// Endpoint inilah yang menjadi sasaran brute force, dan membatasi
	// endpoint lain tanpa alasan hanya mengganggu pemakai yang sah.
	auth.Post("/login", middleware.LoginRateLimiter(), deps.AuthService.Login)

	auth.Post("/refresh", deps.AuthService.Refresh)
	auth.Post("/logout", deps.AuthService.Logout)

	// /me memerlukan access token: pertanyaannya memang "siapa saya".
	auth.Get("/me", middleware.RequireAuth(deps.JWT), deps.AuthService.Me)

	// ============================================================
	// TERKUNCI, wajib membawa access token
	// ============================================================
	//
	// RequireAuth dipasang pada GRUP, bukan pada masing-masing route.
	// Bedanya menentukan: bila dipasang satu per satu, endpoint yang
	// ditambahkan kelak akan terbuka lebar hanya karena seseorang lupa
	// menuliskannya. Dipasang di grup, yang baru ikut terlindungi
	// secara bawaan.
	perms := deps.Permissions

	// ------------------------------------------------------------
	// /users, Langkah 6 Modul 6
	// ------------------------------------------------------------
	users := api.Group("/users",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT), // 1. siapa Anda?  (401)
	)

	// Hak dapat diputuskan tanpa melihat data -> middleware (403).
	users.Get("/",
		middleware.RequirePermission(perms, "user:list"),
		deps.UserService.List)
	users.Delete("/:id",
		middleware.RequirePermission(perms, "user:delete"),
		deps.UserService.Delete)
	users.Patch("/:id/role",
		middleware.RequirePermission(perms, "role:assign"),
		deps.UserService.AssignRole)

	// Hak bergantung pada kepemilikan -> diperiksa di SERVICE.
	// Tidak "telanjang": tetap di bawah RequireAuth milik grup.
	users.Get("/:id", deps.UserService.Get)

	// ------------------------------------------------------------
	// /students, Tugas Mandiri Modul 6
	//
	// File ini adalah PETA HAK AKSES. Urutan middleware penting:
	// RequireAuth (grup) selalu berjalan sebelum RequirePermission.
	// ------------------------------------------------------------
	students := api.Group("/students",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)

	// Hak dapat diputuskan tanpa melihat data -> middleware.
	students.Get("/",
		middleware.RequirePermission(perms, "student:list"),
		deps.StudentService.List)
	students.Post("/",
		middleware.RequirePermission(perms, "student:create"),
		deps.StudentService.Create)
	students.Delete("/:id",
		middleware.RequirePermission(perms, "student:delete"),
		deps.StudentService.Delete)

	// Hak bergantung pada owner_id -> diperiksa di service
	// (StudentService.authorizeStudent + CanAccessStudent).
	students.Get("/:id", deps.StudentService.Get)
	students.Put("/:id", deps.StudentService.Replace)
	students.Patch("/:id", deps.StudentService.Patch)

	// portofolio mahasiswa, disimpan di MongoDB (Tugas Mandiri Modul 10).
	// Hak akses diperiksa di service: membaca cukup login, mengubah harus
	// pemilik data mahasiswa atau pemegang student:update:any.
	students.Get("/:id/portfolio", deps.PortfolioService.Get)
	students.Put("/:id/portfolio", deps.PortfolioService.Put)
	students.Post("/:id/portfolio/projects", deps.PortfolioService.AddProject)
	students.Delete("/:id/portfolio/projects/:projectId", deps.PortfolioService.RemoveProject)

	// ------------------------------------------------------------
	// /courses dan /enrollments, Modul 9 (KRS)
	// ------------------------------------------------------------
	courses := api.Group("/courses",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))

	// "/mine" didaftarkan SEBELUM "/:id". Fiber mencocokkan route sesuai
	// urutan pendaftaran; bila dibalik, "mine" dianggap sebagai :id.
	courses.Get("/", deps.CourseService.List)
	courses.Get("/mine", deps.CourseService.Mine)
	courses.Get("/:id", deps.CourseService.Get)
	courses.Post("/:id/enroll", deps.CourseService.Enroll)
	courses.Delete("/:id/enroll", deps.CourseService.Drop)
	courses.Patch("/:id",
		middleware.RequirePermission(perms, "course:manage"),
		deps.CourseService.UpdateQuota)

	// ulasan kelas, disimpan di MongoDB (Modul 10)
	courses.Get("/:id/reviews", deps.ReviewService.ListByCourse)
	courses.Get("/:id/reviews/summary", deps.ReviewService.Summary)
	courses.Post("/:id/reviews", deps.ReviewService.Create)

	enrollments := api.Group("/enrollments",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))
	enrollments.Post("/swap", deps.CourseService.Swap)

	// ------------------------------------------------------------
	// /supervisors dan /supervisions, Tugas Mandiri Modul 9
	// ------------------------------------------------------------
	supervisors := api.Group("/supervisors",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))

	supervisors.Get("/", deps.SupervisorService.List)
	supervisors.Get("/:id", deps.SupervisorService.Get)
	supervisors.Post("/:id/assign",
		middleware.RequirePermission(perms, "supervision:manage"),
		deps.SupervisorService.Assign)
	supervisors.Patch("/:id",
		middleware.RequirePermission(perms, "supervision:manage"),
		deps.SupervisorService.UpdateQuota)

	supervisions := api.Group("/supervisions",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))
	supervisions.Post("/switch",
		middleware.RequirePermission(perms, "supervision:manage"),
		deps.SupervisorService.Switch)

	// ------------------------------------------------------------
	// /reviews dan /portfolios, Modul 10 (MongoDB)
	// ------------------------------------------------------------
	reviews := api.Group("/reviews",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))
	reviews.Get("/:id", deps.ReviewService.Get)
	reviews.Patch("/:id", deps.ReviewService.Update)
	reviews.Delete("/:id", deps.ReviewService.Delete)
	reviews.Post("/:id/replies", deps.ReviewService.Reply)
	reviews.Post("/:id/helpful", deps.ReviewService.MarkHelpful)

	portfolios := api.Group("/portfolios",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))
	portfolios.Get("/", deps.PortfolioService.Search)
	portfolios.Get("/stats/technologies", deps.PortfolioService.TopTechnologies)

	// ------------------------------------------------------------
	// /notifications dan /admin, Modul 11 (diisi worker, dibaca lewat API)
	// ------------------------------------------------------------
	notifications := api.Group("/notifications",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))
	notifications.Get("/mine", deps.NotificationService.Mine)

	admin := api.Group("/admin",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT))
	admin.Get("/queue",
		middleware.RequirePermission(perms, "queue:monitor"),
		deps.NotificationService.Monitor)
}

// healthCheck memeriksa DUA database. Aplikasi dianggap sehat hanya bila
// keduanya bisa dihubungi, dan pesan kegagalannya menyebut yang mana.
func healthCheck(pool *pgxpool.Pool, mongoClient *mongo.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			return helper.ServiceUnavailable("PostgreSQL tidak dapat dihubungi")
		}
		if err := mongoClient.Ping(ctx, nil); err != nil {
			return helper.ServiceUnavailable("MongoDB tidak dapat dihubungi")
		}
		return helper.Success(c, fiber.StatusOK, "server, PostgreSQL, dan MongoDB berjalan", nil)
	}
}
