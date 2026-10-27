// Alat uji Modul 11 (background job, outbox, idempotency).
//
// Jalankan dari folder proyek, dengan API (go run . -role=api) dan worker
// (go run . -role=worker) menyala di dua terminal lain:
//
//	go run ./tools/uji11 -mode siapkan   daftarkan akun mhsuji dan salahalamat lewat API
//	go run ./tools/uji11 -mode c1        jalur normal
//	go run ./tools/uji11 -mode c2        Redis mati, KRS tetap berjalan (alat menunggu Redis dinyalakan)
//	go run ./tools/uji11 -mode c3        temporary failure (worker dengan MAILER_FAIL_EVERY=3)
//	go run ./tools/uji11 -mode c4        permanent failure (worker dengan MAILER_FAIL_EVERY=0)
//	go run ./tools/uji11 -mode c5        idempotency (mulai dengan worker MATI)
//	go run ./tools/uji11 -mode c6        graceful shutdown (worker dengan MAILER_DELAY_MS=5000)
//	go run ./tools/uji11 -mode kuota     Tugas Mandiri D.2 (worker dengan MAILER_FAIL_EVERY=3)
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/app/repository"
	"github.com/Reverseflash45/api-students-queue/app/service"
	"github.com/Reverseflash45/api-students-queue/config"
	"github.com/Reverseflash45/api-students-queue/database"
	"github.com/Reverseflash45/api-students-queue/helper"
)

const (
	mailLog   = "logs/mail.log"
	workerLog = "logs/worker.log"
	password  = "Rahasia123"
)

var (
	base   string
	ctx    = context.Background()
	client = &http.Client{Timeout: 15 * time.Second}
	pg     *pgxpool.Pool
	jm     *helper.JWTManager

	uji, salah, admin account
)

type account struct {
	ID       int
	Username string
	Token    string
}

func main() {
	mode := flag.String("mode", "c1", "siapkan | c1 | c2 | c3 | c4 | c5 | c6 | kuota")
	flag.StringVar(&base, "base", "http://localhost:3000/api/v1", "alamat dasar API")
	flag.Parse()

	config.LoadEnv()
	var err error
	if pg, err = database.NewPool(ctx); err != nil {
		fmt.Println("gagal terhubung ke PostgreSQL:", err)
		os.Exit(1)
	}
	defer pg.Close()
	jm = helper.NewJWTManager(config.GetEnv("JWT_SECRET", ""),
		config.GetEnv("JWT_ISSUER", "praktikum-backend"), time.Hour)

	if *mode == "siapkan" {
		modeSiapkan()
		return
	}
	uji = mustAccount("mhsuji")
	salah = mustAccount("salahalamat")
	admin = mustAccount("admin_uji")

	switch *mode {
	case "c1":
		modeC1()
	case "c2":
		modeC2()
	case "c3":
		modeC3()
	case "c4":
		modeC4()
	case "c5":
		modeC5()
	case "c6":
		modeC6()
	case "kuota":
		modeKuota()
	default:
		fmt.Println("mode tidak dikenal:", *mode)
		os.Exit(1)
	}
}

// ================================================================
// Akun dan HTTP
// ================================================================

func mustAccount(username string) account {
	var u model.User
	err := pg.QueryRow(ctx, "SELECT id, username, role FROM users WHERE username = $1", username).
		Scan(&u.ID, &u.Username, &u.Role)
	if err != nil {
		fmt.Printf("akun %s belum ada. Jalankan dulu: go run ./tools/uji11 -mode siapkan\n", username)
		os.Exit(1)
	}
	tok, _ := jm.GenerateAccess(u)
	return account{ID: u.ID, Username: u.Username, Token: tok}
}

type reply struct {
	Status  int
	Code    string
	Body    []byte
	Elapsed time.Duration
}

func call(method, path, token string, body any) reply {
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, base+path, rdr)
	if method != http.MethodGet && method != http.MethodDelete {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	t0 := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return reply{Code: "TIDAK_ADA_RESPONS", Elapsed: time.Since(t0)}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(b, &e)
	return reply{Status: resp.StatusCode, Code: e.Code, Body: b, Elapsed: time.Since(t0)}
}

func (r reply) label() string {
	if r.Status >= 200 && r.Status < 300 {
		return fmt.Sprintf("%d", r.Status)
	}
	return fmt.Sprintf("%d %s", r.Status, r.Code)
}

func step(no, desc string, r reply) {
	fmt.Printf("  %-3s %-52s -> %-22s (%v)\n", no, desc, r.label(), r.Elapsed.Round(time.Millisecond))
}

// ================================================================
// Pemeriksaan
// ================================================================

var pass, fail int

func check(name string, ok bool, detail string) {
	mark := "[OK]   "
	if ok {
		pass++
	} else {
		fail++
		mark = "[SALAH]"
	}
	fmt.Printf("%s %s", mark, name)
	if detail != "" {
		fmt.Printf("  =>  %s", detail)
	}
	fmt.Println()
}

func summary() {
	fmt.Printf("\nRINGKASAN: %d pemeriksaan, %d sesuai, %d salah\n", pass+fail, pass, fail)
}

func resetUji() {
	raw, err := os.ReadFile("reset11.sql")
	if err != nil {
		fmt.Println("reset11.sql tidak ditemukan di folder proyek")
		os.Exit(1)
	}
	if _, err := pg.Exec(ctx, string(raw)); err != nil {
		fmt.Println("reset gagal:", err)
		os.Exit(1)
	}
}

func clearMailLog() {
	_ = os.MkdirAll(filepath.Dir(mailLog), 0o755)
	_ = os.WriteFile(mailLog, nil, 0o600)
}

// countLines menghitung baris berkas yang memuat semua potongan teks.
func countLines(path string, from int64, parts ...string) int {
	return len(grepLines(path, from, parts...))
}

func grepLines(path string, from int64, parts ...string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	_, _ = f.Seek(from, io.SeekStart)
	out := []string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		ok := true
		for _, p := range parts {
			if !strings.Contains(line, p) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, line)
		}
	}
	return out
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func countInt(sql string, args ...any) int {
	var n int
	_ = pg.QueryRow(ctx, sql, args...).Scan(&n)
	return n
}

func myNotifications(a account) []string {
	r := call("GET", "/notifications/mine", a.Token, nil)
	var w struct {
		Data []model.Notification `json:"data"`
	}
	_ = json.Unmarshal(r.Body, &w)
	out := []string{}
	for _, n := range w.Data {
		out = append(out, n.Subject)
	}
	return out
}

type queueStats struct {
	OutboxPending int    `json:"outbox_pending"`
	QueueError    string `json:"queue_error"`
	Queue         *struct {
		Pending  int `json:"pending"`
		Retry    int `json:"retry"`
		Archived int `json:"archived"`
	} `json:"queue"`
}

func adminQueue() (queueStats, string) {
	r := call("GET", "/admin/queue", admin.Token, nil)
	var w struct {
		Data queueStats `json:"data"`
	}
	_ = json.Unmarshal(r.Body, &w)
	var pretty bytes.Buffer
	_ = json.Indent(&pretty, r.Body, "  ", "  ")
	return w.Data, pretty.String()
}

func wait(d time.Duration, why string) {
	fmt.Printf("\n... menunggu %v (%s)\n\n", d, why)
	time.Sleep(d)
}

// ================================================================
// siapkan
// ================================================================

func modeSiapkan() {
	fmt.Println("Mendaftarkan akun uji lewat POST /api/v1/auth/register lalu login:")
	for _, a := range []struct{ user, email string }{
		{"mhsuji", "mhsuji@student.unair.ac.id"},
		{"salahalamat", "salah@kampus.invalid"},
	} {
		r := call("POST", "/auth/register", "", map[string]string{
			"username": a.user, "email": a.email, "password": password})
		fmt.Printf("  register %-12s %-30s -> %s\n", a.user, a.email, r.label())
		l := call("POST", "/auth/login", "", map[string]string{"username": a.user, "password": password})
		var w struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		_ = json.Unmarshal(l.Body, &w)
		tok := w.Data.AccessToken
		if len(tok) > 24 {
			tok = tok[:24] + "..."
		}
		fmt.Printf("  login    %-12s -> %s  access_token %s\n", a.user, l.label(), tok)
	}
	fmt.Println("\n201 berarti akun baru dibuat, 409 berarti akun sudah ada. Keduanya boleh.")
	fmt.Println("Token admin dicetak dari akun admin_uji (migrations/007_seed_loadtest.sql).")
}

// ================================================================
// C.1 Jalur normal
// ================================================================

func modeC1() {
	resetUji()
	clearMailLog()
	fmt.Println("C.1 Jalur normal (reset11.sql dijalankan, logs/mail.log dikosongkan)")
	step("1", "POST /courses/2/enroll", call("POST", "/courses/2/enroll", uji.Token, nil))
	step("2", "POST /courses/2/enroll (diulang)", call("POST", "/courses/2/enroll", uji.Token, nil))
	step("3", "DELETE /courses/2/enroll", call("DELETE", "/courses/2/enroll", uji.Token, nil))
	step("4", "POST /courses/2/enroll", call("POST", "/courses/2/enroll", uji.Token, nil))
	step("5", `POST /enrollments/swap {"from_course_id":2,"to_course_id":4}`,
		call("POST", "/enrollments/swap", uji.Token, map[string]int{"from_course_id": 2, "to_course_id": 4}))
	wait(5*time.Second, "worker mengirim notifikasi")

	rows, _ := pg.Query(ctx,
		`SELECT id, event_type FROM outbox WHERE (payload->>'user_id')::INT = $1 ORDER BY id`, uji.ID)
	events := []string{}
	fmt.Println("outbox milik mhsuji:")
	for rows.Next() {
		var id int64
		var ev string
		_ = rows.Scan(&id, &ev)
		events = append(events, ev)
		fmt.Printf("  %d  %s\n", id, ev)
	}
	rows.Close()
	want := []string{model.EventEnrollmentCreated, model.EventEnrollmentDropped, model.EventEnrollmentCreated, model.EventEnrollmentSwapped}
	check("outbox berisi tepat 4 event dengan urutan yang benar",
		strings.Join(events, ",") == strings.Join(want, ","), fmt.Sprintf("%d baris", len(events)))

	subjects := myNotifications(uji)
	fmt.Println("\nGET /notifications/mine (terbaru dulu):")
	for _, s := range subjects {
		fmt.Println("  " + s)
	}
	wantSub := []string{
		"KRS: pindah dari IF102 Basis Data Lanjut ke IF302 Keamanan Informasi",
		"KRS: berhasil terdaftar di IF102 Basis Data Lanjut",
		"KRS: keluar dari IF102 Basis Data Lanjut",
		"KRS: berhasil terdaftar di IF102 Basis Data Lanjut",
	}
	check("4 notifikasi dengan subjek yang benar", strings.Join(subjects, "|") == strings.Join(wantSub, "|"), "")

	n := countLines(mailLog, 0, "mhsuji@")
	check("logs/mail.log berisi tepat 4 baris untuk mhsuji@", n == 4, fmt.Sprintf("%d baris", n))
	summary()
}

// ================================================================
// C.2 Redis mati
// ================================================================

func modeC2() {
	fmt.Println("C.2 Redis mati, KRS tetap berjalan")
	fmt.Println("Pastikan Redis SUDAH DIMATIKAN (memurai.exe --service-stop) sebelum mode ini dijalankan.")
	before := len(myNotifications(uji))
	_, _ = pg.Exec(ctx, "DELETE FROM enrollments WHERE user_id = $1 AND course_id IN (5,6,7)", uji.ID)
	_, _ = pg.Exec(ctx, "UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)")

	ok201 := true
	for _, c := range []int{5, 6, 7} {
		r := call("POST", fmt.Sprintf("/courses/%d/enroll", c), uji.Token, nil)
		step("", fmt.Sprintf("POST /courses/%d/enroll", c), r)
		ok201 = ok201 && r.Status == 201
	}
	check("ketiga pendaftaran 201", ok201, "")
	wait(5*time.Second, "relay mencoba dan gagal karena Redis mati")

	st, raw := adminQueue()
	fmt.Println("GET /admin/queue:\n  " + raw)
	check("outbox_pending 3 dan ada queue_error", st.OutboxPending == 3 && st.QueueError != "",
		fmt.Sprintf("outbox_pending=%d", st.OutboxPending))

	fmt.Println("\n>>> NYALAKAN REDIS SEKARANG (memurai.exe --service-start). Alat ini menunggu sampai 120 detik.")
	t0 := time.Now()
	recovered := time.Duration(0)
	for time.Since(t0) < 120*time.Second {
		time.Sleep(time.Second)
		st, _ = adminQueue()
		if st.QueueError == "" && recovered == 0 {
			recovered = time.Since(t0)
			fmt.Printf("Redis terhubung lagi setelah %v, menunggu outbox kosong...\n", recovered.Round(time.Second))
		}
		if recovered > 0 && st.OutboxPending == 0 {
			break
		}
	}
	time.Sleep(5 * time.Second) // beri waktu worker mengirim email
	fmt.Printf("outbox_pending sekarang: %d\n", st.OutboxPending)
	check("outbox_pending kembali 0 setelah Redis menyala", st.OutboxPending == 0, "")
	after := myNotifications(uji)
	check("GET /notifications/mine bertambah 3", len(after) == before+3,
		fmt.Sprintf("sebelum %d, sesudah %d", before, len(after)))
	summary()
}

// ================================================================
// C.3 Temporary failure
// ================================================================

func modeC3() {
	fmt.Println("C.3 Temporary failure (worker harus dijalankan dengan MAILER_FAIL_EVERY=3)")
	resetUji()
	clearMailLog()
	off := fileSize(workerLog)
	all201 := true
	for c := 1; c <= 6; c++ {
		r := call("POST", fmt.Sprintf("/courses/%d/enroll", c), uji.Token, nil)
		step("", fmt.Sprintf("POST /courses/%d/enroll", c), r)
		all201 = all201 && r.Status == 201
	}
	check("keenam pendaftaran 201", all201, "")
	wait(20*time.Second, "sebagian pengiriman gagal lalu di-retry")

	dbRows := countInt("SELECT COUNT(*) FROM notifications WHERE user_id = $1", uji.ID)
	apiRows := len(myNotifications(uji))
	mailRows := countLines(mailLog, 0, "mhsuji@")
	fmt.Printf("baris notifications mhsuji   : %d\n", dbRows)
	fmt.Printf("GET /notifications/mine      : %d\n", apiRows)
	fmt.Printf("baris logs/mail.log mhsuji@  : %d\n", mailRows)
	check("ketiga angka sama-sama 6", dbRows == 6 && apiRows == 6 && mailRows == 6, "")

	failed := grepLines(workerLog, off, `"task_failed"`, "pengiriman gagal sementara")
	fmt.Printf("\nbaris task_failed di logs/worker.log: %d\n", len(failed))
	for _, l := range failed {
		fmt.Println("  " + shorten(l))
	}
	check("minimal 2 baris task_failed \"pengiriman gagal sementara\"", len(failed) >= 2, "")
	summary()
}

// ================================================================
// C.4 Permanent failure
// ================================================================

func modeC4() {
	fmt.Println("C.4 Permanent failure (worker harus dijalankan dengan MAILER_FAIL_EVERY=0)")
	_, _ = pg.Exec(ctx, "DELETE FROM enrollments WHERE user_id = $1", salah.ID)
	_, _ = pg.Exec(ctx, "UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)")

	before, _ := adminQueue()
	archivedBefore := -1
	if before.Queue != nil {
		archivedBefore = before.Queue.Archived
	}
	fmt.Printf("queue.archived sebelum: %d\n", archivedBefore)
	off := fileSize(workerLog)

	r := call("POST", "/courses/2/enroll", salah.Token, nil)
	step("", "salahalamat POST /courses/2/enroll", r)
	check("pendaftaran 201 walaupun alamat email tidak ada", r.Status == 201, "")
	wait(15*time.Second, "worker mencoba mengirim ke salah@kampus.invalid")

	after, raw := adminQueue()
	fmt.Println("GET /admin/queue:\n  " + raw)
	if after.Queue != nil {
		check("queue.archived bertambah tepat 1", after.Queue.Archived == archivedBefore+1,
			fmt.Sprintf("%d -> %d", archivedBefore, after.Queue.Archived))
		check("queue.retry bernilai 0", after.Queue.Retry == 0, fmt.Sprintf("retry=%d", after.Queue.Retry))
	}
	lines := grepLines(workerLog, off, `"task_failed"`, "kampus.invalid")
	fmt.Printf("\nbaris task_failed yang menyebut kampus.invalid: %d\n", len(lines))
	for _, l := range lines {
		fmt.Println("  " + shorten(l))
	}
	check("tepat satu baris task_failed dengan attempt 1/6",
		len(lines) == 1 && strings.Contains(lines[0], `"attempt":"1/6"`), "")
	summary()
}

// ================================================================
// C.5 Idempotency
// ================================================================

func modeC5() {
	fmt.Println("C.5 Idempotency (worker HARUS dalam keadaan mati saat mode ini mulai)")
	_, _ = pg.Exec(ctx, "DELETE FROM enrollments WHERE user_id = $1 AND course_id = 8", uji.ID)
	_, _ = pg.Exec(ctx, "UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)")

	r := call("POST", "/courses/8/enroll", uji.Token, nil)
	step("1", "POST /courses/8/enroll", r)
	time.Sleep(2 * time.Second)

	var id int64
	var published *time.Time
	_ = pg.QueryRow(ctx, "SELECT id, published_at FROM outbox WHERE id = (SELECT MAX(id) FROM outbox)").Scan(&id, &published)
	fmt.Printf("  2   SELECT MAX(id) FROM outbox -> %d, published_at %v\n", id, published)
	if published != nil {
		fmt.Println("\nEvent sudah di-publish, artinya worker masih menyala. Matikan worker (Ctrl+C) lalu ulangi mode ini.")
		os.Exit(1)
	}

	// Sama dengan: go run ./tools/redeliver -outbox <id> -copies 5
	n := redeliver(id, 5)
	fmt.Printf("  3   redeliver -outbox %d -copies 5 -> %d salinan task masuk Redis\n", id, n)

	mailBefore := countLines(mailLog, 0, "IF306")
	off := fileSize(workerLog)
	fmt.Println("\n>>> NYALAKAN WORKER SEKARANG (go run . -role=worker). Alat ini menunggu sampai 120 detik.")
	t0 := time.Now()
	for time.Since(t0) < 120*time.Second {
		time.Sleep(time.Second)
		if countInt("SELECT COUNT(*) FROM notifications WHERE outbox_id = $1", id) > 0 {
			break
		}
	}
	wait(8*time.Second, "seluruh salinan dikerjakan")

	notif := countInt("SELECT COUNT(*) FROM notifications WHERE outbox_id = $1", id)
	mail := countLines(mailLog, 0, "IF306") - mailBefore
	idTag := fmt.Sprintf(`"outbox_id":%d,`, id)
	falseLines := countLines(workerLog, off, `"notification_processed"`, idTag, `"already_sent":false`)
	trueLines := countLines(workerLog, off, `"notification_processed"`, idTag, `"already_sent":true`)
	fmt.Printf("SELECT COUNT(*) FROM notifications WHERE outbox_id = %d -> %d\n", id, notif)
	fmt.Printf("baris logs/mail.log yang bertambah untuk IF306       -> %d\n", mail)
	fmt.Printf("notification_processed already_sent=false            -> %d\n", falseLines)
	fmt.Printf("notification_processed already_sent=true             -> %d\n", trueLines)
	check("tepat 1 baris notifications", notif == 1, "")
	check("logs/mail.log hanya bertambah 1 baris", mail == 1, "")
	check("6 baris notification_processed: 1 false, 5 true", falseLines == 1 && trueLines == 5, "")
	summary()
}

// redeliver sama isinya dengan tools/redeliver: membuat salinan task tanpa
// TaskID, jadi Redis menerimanya sebagai task-task yang berbeda.
func redeliver(id int64, copies int) int {
	e, err := repository.NewOutboxRepository(pg).FindByID(ctx, id)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	q := asynq.NewClient(config.RedisOpt())
	defer q.Close()
	n := 0
	for i := 0; i < copies; i++ {
		task, err := service.NewNotificationTask(e)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if _, err := q.EnqueueContext(ctx, task); err == nil {
			n++
		} else {
			fmt.Println("  enqueue gagal:", err)
		}
	}
	return n
}

// ================================================================
// C.6 Graceful shutdown
// ================================================================

func modeC6() {
	fmt.Println("C.6 Graceful shutdown (worker harus dijalankan dengan MAILER_DELAY_MS=5000)")
	_, _ = pg.Exec(ctx, "DELETE FROM enrollments WHERE user_id = $1 AND course_id = 9", uji.ID)
	_, _ = pg.Exec(ctx, "UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)")
	r := call("POST", "/courses/9/enroll", uji.Token, nil)
	step("", "POST /courses/9/enroll", r)
	fmt.Println("\n>>> Begitu log relay_done muncul di terminal worker, tunggu sekitar 2 detik lalu tekan Ctrl+C di sana.")
	fmt.Println("    Alat ini menunggu notifikasi kelas 9 sampai 60 detik.")
	t0 := time.Now()
	found := false
	for time.Since(t0) < 60*time.Second {
		time.Sleep(time.Second)
		for _, s := range myNotifications(uji) {
			if strings.Contains(s, "IF307") {
				found = true
			}
		}
		if found {
			break
		}
	}
	check("notifikasi untuk kelas 9 (IF307) ada", found, "")
	fmt.Println("Periksa juga terminal worker: notification_processed muncul SEBELUM \"aplikasi berhenti dengan rapi\".")
	summary()
}

// ================================================================
// D.2 Notifikasi perubahan kuota
// ================================================================

func modeKuota() {
	fmt.Println("D.2 Notifikasi perubahan kuota (worker dengan MAILER_FAIL_EVERY=3)")
	const courseID = 10 // IF308 Visi Komputer
	var code string
	_ = pg.QueryRow(ctx, "SELECT code FROM courses WHERE id = $1", courseID).Scan(&code)

	// Peserta disiapkan langsung lewat SQL supaya tidak ikut menghasilkan
	// event pendaftaran: yang diuji di sini hanya event perubahan kuota.
	_, _ = pg.Exec(ctx, "DELETE FROM enrollments WHERE course_id = $1", courseID)
	names := []string{"mhs001", "mhs002", "mhs003", "mhs004", "mhs005", "mhs006", "salahalamat"}
	_, _ = pg.Exec(ctx, `INSERT INTO enrollments (course_id, user_id)
		SELECT $1, id FROM users WHERE username = ANY($2)`, courseID, names)
	_, _ = pg.Exec(ctx, "UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)")
	clearMailLog()
	fmt.Printf("peserta %s: %s (salahalamat sengaja ikut, alamatnya tidak ada)\n", code, strings.Join(names, ", "))

	g := call("GET", fmt.Sprintf("/courses/%d", courseID), admin.Token, nil)
	var w struct {
		Data model.Course `json:"data"`
	}
	_ = json.Unmarshal(g.Body, &w)
	newQuota := w.Data.Quota + 1
	off := fileSize(workerLog)
	r := call("PATCH", fmt.Sprintf("/courses/%d", courseID), admin.Token,
		map[string]int{"quota": newQuota, "version": w.Data.Version})
	step("", fmt.Sprintf("PATCH /courses/%d {quota:%d, version:%d}", courseID, newQuota, w.Data.Version), r)

	var id int64
	_ = pg.QueryRow(ctx, "SELECT MAX(id) FROM outbox WHERE event_type = $1", model.EventCourseQuotaChanged).Scan(&id)
	var payload string
	_ = pg.QueryRow(ctx, "SELECT payload::text FROM outbox WHERE id = $1", id).Scan(&payload)
	fmt.Printf("outbox %d: %s\n", id, payload)

	t0 := time.Now()
	for time.Since(t0) < 90*time.Second {
		time.Sleep(time.Second)
		if countLines(workerLog, off, `"notification_processed"`, fmt.Sprintf(`"outbox_id":%d,`, id)) > 0 {
			break
		}
	}
	fmt.Printf("\nlog worker untuk outbox %d:\n", id)
	for _, l := range grepLines(workerLog, off, fmt.Sprintf(`"outbox_id":%d,`, id)) {
		fmt.Println("  " + shorten(l))
	}
	for _, l := range grepLines(workerLog, off, `"task_failed"`) {
		fmt.Println("  " + shorten(l))
	}

	report := func(title string) bool {
		fmt.Println("\n" + title)
		allOne := true
		for _, n := range names {
			var email string
			var uid int
			_ = pg.QueryRow(ctx, "SELECT id, email FROM users WHERE username = $1", n).Scan(&uid, &email)
			rows := countInt("SELECT COUNT(*) FROM notifications WHERE outbox_id = $1 AND user_id = $2", id, uid)
			mails := countLines(mailLog, 0, "to="+email+" ", "kuota "+code)
			want := 1
			if strings.HasSuffix(email, ".invalid") {
				want = 0
			}
			fmt.Printf("  %-12s %-30s notifications=%d  mail.log=%d\n", n, email, rows, mails)
			allOne = allOne && rows == want && mails == want
		}
		return allOne
	}
	check("setiap peserta menerima tepat 1 email (salahalamat 0)", report("Hasil setelah fan-out:"), "")

	n := redeliver(id, 3)
	fmt.Printf("\nredeliver -outbox %d -copies 3 -> %d salinan\n", id, n)
	wait(12*time.Second, "salinan dikerjakan worker")
	check("setelah 3 salinan tambahan, tetap tepat 1 email per peserta", report("Hasil setelah redeliver:"), "")
	summary()
}

func shorten(s string) string {
	if len(s) > 230 {
		return s[:227] + "..."
	}
	return s
}
