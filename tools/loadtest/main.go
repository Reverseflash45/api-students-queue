// Alat uji beban dan uji fungsional untuk Modul 9.
//
// Folder tools/ bukan bagian aplikasi: alat ini dipakai untuk membuktikan
// ada atau tidaknya race condition, dan tidak ikut di-deploy.
//
// Server harus sedang berjalan di terminal lain. Jalankan dari folder
// proyek supaya .env terbaca:
//
//	go run ./tools/loadtest -mode crowd            (C.1)
//	go run ./tools/loadtest -mode credits          (C.2)
//	go run ./tools/loadtest -mode swap             (C.3)
//	go run ./tools/loadtest -mode fungsional       (C.4 dan D.2)
//	go run ./tools/loadtest -mode sehat            (C.5)
//	go run ./tools/loadtest -mode pembimbing       (D.3)
//	go run ./tools/loadtest -mode dobelpembimbing  (D.2 butir 2)
//	go run ./tools/loadtest -mode gantibalik       (D.2 butir 3)
//	go run ./tools/loadtest -mode gantiganda       (D.4 nomor 1)
//
// Flag -runs N mengulang uji N kali, dengan reset sebelum setiap percobaan.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/config"
	"github.com/Reverseflash45/api-students-queue/database"
	"github.com/Reverseflash45/api-students-queue/helper"
)

var (
	base   string
	client = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        300,
			MaxIdleConnsPerHost: 300,
		},
	}
	db  *pgxpool.Pool
	ctx = context.Background()

	students   []account // mhs001 .. mhs060
	admin      account
	courseID   = map[string]int{}
	supID      = map[string]int{}
	thesisStud []int // id students dengan NIM 999xxxxxx
)

type account struct {
	ID       int
	Username string
	Token    string
}

// fire menjalankan n pekerjaan secara serentak dan menghitung hasilnya.
//
// Setiap goroutine menunggu di depan "gerbang" start yang sama. Gerbang
// baru dibuka setelah SELURUH goroutine siap, sehingga request benar-benar
// berangkat hampir bersamaan, bukan bergiliran sesuai urutan dibuatnya.
func fire(n int, job func(i int) string) map[string]int {
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int, out chan<- string) {
			defer wg.Done()
			<-start
			out <- job(i)
		}(i, results)
	}

	close(start) // buka gerbang
	wg.Wait()    // tunggu SEMUA goroutine selesai
	close(results)

	tally := map[string]int{}
	for r := range results {
		tally[r]++
	}
	return tally
}

func main() {
	mode := flag.String("mode", "crowd", "crowd | credits | swap | fungsional | sehat | pembimbing | dobelpembimbing | gantibalik | gantiganda")
	runs := flag.Int("runs", 1, "berapa kali uji diulang (reset sebelum setiap percobaan)")
	flag.StringVar(&base, "base", "http://localhost:3000/api/v1", "alamat dasar API")
	flag.Parse()

	config.LoadEnv()

	var err error
	db, err = database.NewPool(ctx)
	if err != nil {
		fmt.Println("gagal terhubung ke database:", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := prepare(); err != nil {
		fmt.Println("persiapan gagal:", err)
		os.Exit(1)
	}

	for r := 1; r <= *runs; r++ {
		if *runs > 1 {
			fmt.Printf("\n===== percobaan %d dari %d =====\n", r, *runs)
		}
		switch *mode {
		case "crowd":
			mustReset()
			modeCrowd()
		case "credits":
			mustReset()
			modeCredits()
		case "swap":
			mustReset()
			modeSwap()
		case "fungsional":
			mustReset()
			modeFungsional()
		case "sehat":
			mustReset()
			modeSehat()
		case "pembimbing":
			mustReset()
			modePembimbing()
		case "dobelpembimbing":
			mustReset()
			modeDobelPembimbing()
		case "gantibalik":
			mustReset()
			modeGantiBalik()
		case "gantiganda":
			mustReset()
			modeGantiGanda()
		default:
			fmt.Println("mode tidak dikenal:", *mode)
			os.Exit(1)
		}
	}
}

// ================================================================
// Persiapan: akun, token, dan id
// ================================================================

func prepare() error {
	secret := config.GetEnv("JWT_SECRET", "")
	if len(secret) < 32 {
		return fmt.Errorf("JWT_SECRET di .env kosong atau terlalu pendek")
	}
	// Alat ini mencetak token sendiri karena ia memegang JWT_SECRET.
	// Itulah sebabnya JWT_SECRET tidak boleh bocor: siapa pun yang
	// memegangnya bisa mencetak token atas nama siapa saja.
	jwtManager := helper.NewJWTManager(secret,
		config.GetEnv("JWT_ISSUER", "praktikum-backend"), time.Hour)

	rows, err := db.Query(ctx,
		"SELECT id, username, role FROM users WHERE username LIKE 'mhs%' OR username = 'admin_uji' ORDER BY username")
	if err != nil {
		return err
	}
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			return err
		}
		token, err := jwtManager.GenerateAccess(u)
		if err != nil {
			return err
		}
		a := account{ID: u.ID, Username: u.Username, Token: token}
		if u.Username == "admin_uji" {
			admin = a
		} else {
			students = append(students, a)
		}
	}
	rows.Close()
	if len(students) < 60 || admin.ID == 0 {
		return fmt.Errorf("akun uji belum ada, jalankan migrations/007_seed_loadtest.sql")
	}

	if err := loadMap("SELECT code, id FROM courses", courseID); err != nil {
		return err
	}
	if err := loadMap("SELECT name, id FROM supervisors", supID); err != nil {
		return err
	}

	rows, err = db.Query(ctx, "SELECT id FROM students WHERE nim LIKE '999%' ORDER BY nim")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		thesisStud = append(thesisStud, id)
	}
	rows.Close()
	return nil
}

func loadMap(sql string, m map[string]int) error {
	rows, err := db.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v int
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		m[k] = v
	}
	return rows.Err()
}

// mustReset menjalankan isi tools/reset.sql, lalu mengembalikan kuota
// bawaan kelas dan pembimbing yang mungkin diubah uji sebelumnya.
func mustReset() {
	stmts := []string{
		"DELETE FROM enrollments WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'mhs%')",
		"UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)",
		`UPDATE courses SET quota = GREATEST(enrolled_count, CASE WHEN code = 'IF399' THEN 5 ELSE 40 END)
		 WHERE code IN ('IF101','IF102','IF301','IF302','IF303','IF304','IF305',
		                'IF306','IF307','IF308','IF309','IF310','IF399')`,
		"DELETE FROM supervisions WHERE student_id IN (SELECT id FROM students WHERE nim LIKE '999%')",
		"UPDATE supervisors s SET supervised_count = (SELECT COUNT(*) FROM supervisions v WHERE v.supervisor_id = s.id)",
		`UPDATE supervisors SET quota = GREATEST(supervised_count, CASE name
			WHEN 'Dosen Pembimbing A' THEN 20 WHEN 'Dosen Pembimbing B' THEN 20
			WHEN 'Dosen Pembimbing C' THEN 1  WHEN 'Dosen Rebutan' THEN 3 END)
		 WHERE name IN ('Dosen Pembimbing A', 'Dosen Pembimbing B', 'Dosen Pembimbing C', 'Dosen Rebutan')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(ctx, s); err != nil {
			fmt.Println("reset gagal:", err)
			os.Exit(1)
		}
	}
}

// ================================================================
// HTTP
// ================================================================

type reply struct {
	Status  int
	Code    string
	Body    []byte
	Elapsed time.Duration
}

func call(method, path, token string, body any) reply {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rdr)
	// RequireJSON menolak POST/PATCH tanpa Content-Type JSON, termasuk
	// POST /courses/:id/enroll yang memang tidak punya body.
	if method != http.MethodGet && method != http.MethodDelete {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	t0 := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return reply{Status: 0, Code: "TIDAK_ADA_RESPONS", Elapsed: time.Since(t0)}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	r := reply{Status: resp.StatusCode, Body: b, Elapsed: time.Since(t0)}

	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(b, &e)
	r.Code = e.Code
	return r
}

// label mengubah response menjadi satu kunci hitungan, misalnya
// "201 OK" atau "409 COURSE_FULL".
func (r reply) label() string {
	if r.Status >= 200 && r.Status < 300 {
		return fmt.Sprintf("%d OK", r.Status)
	}
	if r.Status == 0 {
		return "--- " + r.Code
	}
	return fmt.Sprintf("%d %s", r.Status, r.Code)
}

func printTally(t map[string]int) {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("hasil request:")
	for _, k := range keys {
		fmt.Printf("  %-30s %d\n", k, t[k])
	}
}

func verdict(ok bool, butir string) {
	if ok {
		fmt.Println("=> SESUAI " + butir)
	} else {
		fmt.Println("=> TIDAK SESUAI " + butir)
	}
}

// ================================================================
// Pemeriksaan isi database
// ================================================================

// courseState membandingkan penghitung dengan jumlah baris yang
// sebenarnya. Penghitung adalah turunan; baris enrollments adalah
// sumber kebenarannya.
func courseState(code string) (quota, count, rows int, consistent bool) {
	_ = db.QueryRow(ctx,
		`SELECT c.quota, c.enrolled_count,
		        (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)
		 FROM courses c WHERE c.code = $1`, code).Scan(&quota, &count, &rows)
	return quota, count, rows, count == rows && count <= quota
}

func printCourse(code string) bool {
	q, c, r, ok := courseState(code)
	status := "KONSISTEN"
	if !ok {
		status = "TIDAK KONSISTEN"
	}
	fmt.Printf("%s kuota=%d enrolled_count=%d baris enrollments=%d -> %s\n", code, q, c, r, status)
	return ok
}

func supervisorState(name string) (quota, count, rows int, consistent bool) {
	_ = db.QueryRow(ctx,
		`SELECT s.quota, s.supervised_count,
		        (SELECT COUNT(*) FROM supervisions v WHERE v.supervisor_id = s.id)
		 FROM supervisors s WHERE s.name = $1`, name).Scan(&quota, &count, &rows)
	return quota, count, rows, count == rows && count <= quota
}

func printSupervisor(name string) bool {
	q, c, r, ok := supervisorState(name)
	status := "KONSISTEN"
	if !ok {
		status = "TIDAK KONSISTEN"
	}
	fmt.Printf("%-18s kuota=%d supervised_count=%d baris supervisions=%d -> %s\n", name, q, c, r, status)
	return ok
}

func deadlockCount() int64 {
	var n int64
	_, _ = db.Exec(ctx, "SELECT pg_stat_clear_snapshot()")
	_ = db.QueryRow(ctx,
		"SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()").Scan(&n)
	return n
}

// ================================================================
// C.1 crowd: 60 mahasiswa berebut IF399 (kuota 5)
// ================================================================

func runCrowd() map[string]int {
	id := courseID["IF399"]
	return fire(60, func(i int) string {
		return call("POST", fmt.Sprintf("/courses/%d/enroll", id), students[i].Token, nil).label()
	})
}

func modeCrowd() {
	fmt.Println("60 mahasiswa berebut kelas IF399 secara serentak...")
	t0 := time.Now()
	tally := runCrowd()
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))
	ok := printCourse("IF399")
	verdict(ok && tally["201 OK"] == 5 && tally["409 COURSE_FULL"] == 55, "C.1")
}

// ================================================================
// C.2 credits: 10 mahasiswa x 12 kelas 3 SKS (batas 24 SKS)
// ================================================================

var twelve = []string{"IF101", "IF102", "IF301", "IF302", "IF303", "IF304",
	"IF305", "IF306", "IF307", "IF308", "IF309", "IF310"}

func modeCredits() {
	fmt.Println("10 mahasiswa, masing-masing mendaftar ke 12 kelas 3 SKS secara serentak (batas 24 SKS)...")
	attackers := students[40:50] // mhs041 .. mhs050
	t0 := time.Now()
	tally := fire(len(attackers)*len(twelve), func(i int) string {
		a := attackers[i/len(twelve)]
		c := courseID[twelve[i%len(twelve)]]
		return call("POST", fmt.Sprintf("/courses/%d/enroll", c), a.Token, nil).label()
	})
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))

	over := []string{}
	for _, a := range attackers {
		var total int
		_ = db.QueryRow(ctx,
			`SELECT COALESCE(SUM(c.credits),0) FROM enrollments e
			 JOIN courses c ON c.id = e.course_id WHERE e.user_id = $1`, a.ID).Scan(&total)
		if total > 24 {
			over = append(over, fmt.Sprintf("%s=%d SKS", a.Username, total))
		}
	}
	allConsistent := true
	for _, code := range twelve {
		if _, _, _, ok := courseState(code); !ok {
			allConsistent = false
		}
	}
	if len(over) == 0 {
		fmt.Println("seluruh 10 mahasiswa <= 24 SKS -> KONSISTEN")
	} else {
		fmt.Printf("%d mahasiswa melebihi 24 SKS: %s -> TIDAK KONSISTEN\n", len(over), strings.Join(over, ", "))
	}
	if !allConsistent {
		fmt.Println("enrolled_count salah satu kelas berbeda dengan jumlah baris -> TIDAK KONSISTEN")
	}
	verdict(len(over) == 0 && allConsistent &&
		tally["201 OK"] == 80 && tally["409 CREDIT_LIMIT_EXCEEDED"] == 40, "C.2")
}

// ================================================================
// C.3 swap: 20 pasang pindah berlawanan arah IF101 <-> IF102
// ================================================================

func seedEnroll(code string, accs []account) {
	for _, a := range accs {
		_, _ = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2)", courseID[code], a.ID)
	}
	_, _ = db.Exec(ctx, `UPDATE courses c SET enrolled_count =
		(SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id) WHERE c.code = $1`, code)
}

func modeSwap() {
	groupA := students[0:20]  // mulai di IF101
	groupB := students[20:40] // mulai di IF102
	seedEnroll("IF101", groupA)
	seedEnroll("IF102", groupB)
	time.Sleep(1500 * time.Millisecond)
	before := deadlockCount()

	fmt.Println("20 pasang mahasiswa pindah kelas berlawanan arah secara serentak...")
	t0 := time.Now()
	tally := fire(40, func(i int) string {
		if i < 20 {
			return call("POST", "/enrollments/swap", groupA[i].Token, swapBody(courseID["IF101"], courseID["IF102"])).label()
		}
		return call("POST", "/enrollments/swap", groupB[i-20].Token, swapBody(courseID["IF102"], courseID["IF101"])).label()
	})
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))

	// Statistik PostgreSQL dikirim backend paling cepat sekitar 1 detik
	// setelah ia menganggur, jadi ditunggu sebentar sebelum dibaca.
	time.Sleep(1500 * time.Millisecond)
	dl := deadlockCount() - before
	fmt.Printf("deadlock terdeteksi PostgreSQL selama uji: %d\n", dl)
	ok1 := printCourse("IF101")
	ok2 := printCourse("IF102")
	verdict(ok1 && ok2 && dl == 0 && tally["200 OK"] == 40 && elapsed < time.Second, "C.3")
}

func swapBody(from, to int) map[string]int {
	return map[string]int{"from_course_id": from, "to_course_id": to}
}

// ================================================================
// C.5 sehat: 3 x crowd tanpa reset, lalu periksa health dan koneksi
// ================================================================

func modeSehat() {
	failed := 0
	for k := 1; k <= 3; k++ {
		tally := runCrowd()
		f := 0
		for label, n := range tally {
			if !strings.HasPrefix(label, "2") {
				f += n
			}
		}
		failed += f
		keys := make([]string, 0, len(tally))
		for label := range tally {
			keys = append(keys, label)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, label := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", label, tally[label]))
		}
		fmt.Printf("crowd ke-%d: %s\n", k, strings.Join(parts, ", "))
	}
	fmt.Printf("total request gagal: %d\n", failed)

	h := call("GET", "/health", "", nil)
	fmt.Printf("GET /health -> %d %.6fs\n", h.Status, h.Elapsed.Seconds())

	rows, err := db.Query(ctx,
		`SELECT pid, state, COALESCE((now() - xact_start)::text, ''), left(query, 60)
		 FROM pg_stat_activity
		 WHERE datname = current_database() AND state <> 'idle' AND pid <> pg_backend_pid()`)
	if err != nil {
		fmt.Println("kueri pemantau gagal:", err)
		return
	}
	idleInTx := 0
	n := 0
	fmt.Println("kueri pemantau B.0 (tanpa koneksi alat ini sendiri):")
	for rows.Next() {
		var pid int
		var state, lama, q string
		_ = rows.Scan(&pid, &state, &lama, &q)
		fmt.Printf("  %d | %s | %s | %s\n", pid, state, lama, q)
		if state == "idle in transaction" {
			idleInTx++
		}
		n++
	}
	rows.Close()
	fmt.Printf("  (%d baris, %d idle in transaction)\n", n, idleInTx)
	verdict(h.Status == 200 && idleInTx == 0, "C.5")
}

// ================================================================
// C.4 dan D.2: uji fungsional satu per satu
// ================================================================

var pass, fail int

func check(name string, ok bool, detail string) {
	if ok {
		pass++
		fmt.Printf("[OK]    %-58s %s\n", name, detail)
	} else {
		fail++
		fmt.Printf("[SALAH] %-58s %s\n", name, detail)
	}
}

func dataVersion(b []byte) int {
	var w struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &w)
	return w.Data.Version
}

func krs(a account) []string {
	r := call("GET", "/courses/mine", a.Token, nil)
	var w struct {
		Data struct {
			Courses []model.Course `json:"courses"`
		} `json:"data"`
	}
	_ = json.Unmarshal(r.Body, &w)
	out := []string{}
	for _, c := range w.Data.Courses {
		out = append(out, c.Code)
	}
	return out
}

func modeFungsional() {
	pass, fail = 0, 0
	m := students
	id := courseID

	fmt.Println("--- C.4 KRS ---")
	r := call("POST", fmt.Sprintf("/courses/%d/enroll", id["IF101"]), m[0].Token, nil)
	check("Daftar ke kelas yang masih kosong", r.Status == 201, r.label())

	r = call("POST", fmt.Sprintf("/courses/%d/enroll", id["IF101"]), m[0].Token, nil)
	_, cnt, rows, _ := courseState("IF101")
	check("Daftar ke kelas yang sama sekali lagi", r.Status == 409 && r.Code == "ALREADY_ENROLLED" && cnt == 1 && rows == 1,
		fmt.Sprintf("%s, IF101 enrolled_count=%d baris=%d", r.label(), cnt, rows))

	for i := 1; i <= 5; i++ {
		call("POST", fmt.Sprintf("/courses/%d/enroll", id["IF399"]), m[i].Token, nil)
	}
	r = call("POST", fmt.Sprintf("/courses/%d/enroll", id["IF399"]), m[6].Token, nil)
	check("Mahasiswa ke-6 mendaftar ke IF399 (kuota 5)", r.Status == 409 && r.Code == "COURSE_FULL", r.label())

	seedEnroll("IF102", m[50:52]) // supaya enrolled_count IF102 tidak nol
	_, before, _, _ := courseState("IF102")
	r = call("DELETE", fmt.Sprintf("/courses/%d/enroll", id["IF102"]), m[7].Token, nil)
	_, after, _, _ := courseState("IF102")
	check("Batalkan kelas yang tidak pernah diambil", r.Status == 409 && r.Code == "NOT_ENROLLED" && before == after,
		fmt.Sprintf("%s, IF102 enrolled_count %d -> %d", r.label(), before, after))

	call("POST", fmt.Sprintf("/courses/%d/enroll", id["IF101"]), m[8].Token, nil)
	r = call("POST", "/enrollments/swap", m[8].Token, swapBody(id["IF101"], id["IF399"]))
	k := krs(m[8])
	check("Pindah ke kelas yang penuh", r.Status == 409 && r.Code == "COURSE_FULL" && len(k) == 1 && k[0] == "IF101",
		fmt.Sprintf("%s, KRS %s setelahnya: %v", r.label(), m[8].Username, k))

	r = call("POST", "/enrollments/swap", m[8].Token, swapBody(id["IF101"], id["IF102"]))
	ok1 := printQuiet("IF101")
	ok2 := printQuiet("IF102")
	check("Pindah ke kelas yang masih kosong", r.Status == 200 && r.Elapsed < time.Second && ok1 && ok2,
		fmt.Sprintf("%s dalam %v, kedua kelas konsisten=%v", r.label(), r.Elapsed.Round(time.Millisecond), ok1 && ok2))

	r = call("POST", "/enrollments/swap", m[8].Token, swapBody(id["IF102"], id["IF102"]))
	check("Pindah dengan from_course_id = to_course_id", r.Status == 422 && r.Code == "VALIDATION_ERROR", r.label())

	g := call("GET", fmt.Sprintf("/courses/%d", id["IF301"]), admin.Token, nil)
	v := dataVersion(g.Body)
	r = call("PATCH", fmt.Sprintf("/courses/%d", id["IF301"]), admin.Token, map[string]int{"quota": 41, "version": v})
	nv := dataVersion(r.Body)
	check("Admin ubah kuota dengan version terbaru", r.Status == 200 && nv == v+1,
		fmt.Sprintf("%s, version %d -> %d", r.label(), v, nv))

	r = call("PATCH", fmt.Sprintf("/courses/%d", id["IF301"]), admin.Token, map[string]int{"quota": 42, "version": v})
	check("Admin ubah kuota dengan version lama", r.Status == 409 && r.Code == "VERSION_CONFLICT",
		fmt.Sprintf("%s (mengirim version %d)", r.label(), v))

	g = call("GET", fmt.Sprintf("/courses/%d", id["IF399"]), admin.Token, nil)
	r = call("PATCH", fmt.Sprintf("/courses/%d", id["IF399"]), admin.Token, map[string]int{"quota": 2, "version": dataVersion(g.Body)})
	check("Admin menurunkan kuota di bawah jumlah peserta", r.Status == 409 && r.Code == "QUOTA_BELOW_ENROLLED", r.label())

	r = call("PATCH", fmt.Sprintf("/courses/%d", id["IF301"]), m[0].Token, map[string]int{"quota": 10, "version": nv})
	check("Mahasiswa mencoba mengubah kuota", r.Status == 403, r.label())

	// kembalikan kuota IF301 ke 40
	g = call("GET", fmt.Sprintf("/courses/%d", id["IF301"]), admin.Token, nil)
	call("PATCH", fmt.Sprintf("/courses/%d", id["IF301"]), admin.Token, map[string]int{"quota": 40, "version": dataVersion(g.Body)})

	fmt.Println("--- D.2 pembimbing tugas akhir ---")
	sA, sB, sC := supID["Dosen Pembimbing A"], supID["Dosen Pembimbing B"], supID["Dosen Pembimbing C"]
	st := thesisStud

	r = call("POST", fmt.Sprintf("/supervisors/%d/assign", sA), admin.Token, map[string]int{"student_id": st[0]})
	check("Tetapkan pembimbing A untuk mahasiswa uji 01", r.Status == 201, r.label())

	r = call("POST", fmt.Sprintf("/supervisors/%d/assign", sB), admin.Token, map[string]int{"student_id": st[0]})
	_, cntB, _, _ := supervisorState("Dosen Pembimbing B")
	check("Tetapkan pembimbing kedua untuk mahasiswa yang sama", r.Status == 409 && r.Code == "ALREADY_SUPERVISED" && cntB == 0,
		fmt.Sprintf("%s, supervised_count B=%d", r.label(), cntB))

	call("POST", fmt.Sprintf("/supervisors/%d/assign", sC), admin.Token, map[string]int{"student_id": st[1]})
	r = call("POST", fmt.Sprintf("/supervisors/%d/assign", sC), admin.Token, map[string]int{"student_id": st[2]})
	check("Tetapkan ke pembimbing yang penuh (C, kuota 1)", r.Status == 409 && r.Code == "SUPERVISOR_FULL", r.label())

	r = call("POST", "/supervisions/switch", admin.Token, map[string]int{"student_id": st[0], "to_supervisor_id": sC})
	var cur int
	_ = db.QueryRow(ctx, "SELECT supervisor_id FROM supervisions WHERE student_id = $1", st[0]).Scan(&cur)
	check("Ganti ke pembimbing yang penuh", r.Status == 409 && r.Code == "SUPERVISOR_FULL" && cur == sA,
		fmt.Sprintf("%s, pembimbing setelahnya tetap A=%v", r.label(), cur == sA))

	r = call("POST", "/supervisions/switch", admin.Token, map[string]int{"student_id": st[0], "to_supervisor_id": sB})
	okA := supQuiet("Dosen Pembimbing A")
	okB := supQuiet("Dosen Pembimbing B")
	check("Ganti ke pembimbing yang masih kosong", r.Status == 200 && r.Elapsed < time.Second && okA && okB,
		fmt.Sprintf("%s dalam %v, A dan B konsisten=%v", r.label(), r.Elapsed.Round(time.Millisecond), okA && okB))

	r = call("POST", "/supervisions/switch", admin.Token, map[string]int{"student_id": st[0], "to_supervisor_id": sB})
	check("Ganti ke pembimbing yang sama dengan sekarang", r.Status == 409 && r.Code == "SAME_SUPERVISOR", r.label())

	r = call("POST", "/supervisions/switch", admin.Token, map[string]int{"student_id": st[5], "to_supervisor_id": sB})
	check("Ganti pembimbing mahasiswa yang belum punya", r.Status == 409 && r.Code == "NOT_SUPERVISED", r.label())

	g = call("GET", fmt.Sprintf("/supervisors/%d", sA), admin.Token, nil)
	v = dataVersion(g.Body)
	r = call("PATCH", fmt.Sprintf("/supervisors/%d", sA), admin.Token, map[string]int{"quota": 21, "version": v})
	nv = dataVersion(r.Body)
	check("Admin ubah kuota pembimbing dengan version terbaru", r.Status == 200 && nv == v+1,
		fmt.Sprintf("%s, version %d -> %d", r.label(), v, nv))

	r = call("PATCH", fmt.Sprintf("/supervisors/%d", sA), admin.Token, map[string]int{"quota": 22, "version": v})
	check("Admin ubah kuota pembimbing dengan version lama", r.Status == 409 && r.Code == "VERSION_CONFLICT", r.label())

	call("POST", fmt.Sprintf("/supervisors/%d/assign", sB), admin.Token, map[string]int{"student_id": st[3]})
	g = call("GET", fmt.Sprintf("/supervisors/%d", sB), admin.Token, nil)
	r = call("PATCH", fmt.Sprintf("/supervisors/%d", sB), admin.Token, map[string]int{"quota": 1, "version": dataVersion(g.Body)})
	check("Admin menurunkan kuota di bawah jumlah bimbingan", r.Status == 409 && r.Code == "QUOTA_BELOW_SUPERVISED", r.label())

	r = call("POST", fmt.Sprintf("/supervisors/%d/assign", sA), m[0].Token, map[string]int{"student_id": st[9]})
	check("Mahasiswa mencoba menetapkan pembimbing", r.Status == 403, r.label())

	r = call("POST", fmt.Sprintf("/supervisors/%d/assign", sA), admin.Token, map[string]int{"student_id": 99999999})
	check("Tetapkan untuk mahasiswa yang tidak ada", r.Status == 404, r.label())

	// kembalikan kuota A ke 20
	g = call("GET", fmt.Sprintf("/supervisors/%d", sA), admin.Token, nil)
	call("PATCH", fmt.Sprintf("/supervisors/%d", sA), admin.Token, map[string]int{"quota": 20, "version": dataVersion(g.Body)})

	fmt.Printf("\nRINGKASAN: %d uji, %d sesuai, %d salah\n", pass+fail, pass, fail)
}

func printQuiet(code string) bool {
	_, _, _, ok := courseState(code)
	return ok
}

func supQuiet(name string) bool {
	_, _, _, ok := supervisorState(name)
	return ok
}

// ================================================================
// D.3 pembimbing: 30 penetapan serentak ke pembimbing berkuota 3
// ================================================================

func modePembimbing() {
	sid := supID["Dosen Rebutan"]
	fmt.Println("30 mahasiswa ditetapkan serentak ke Dosen Rebutan (kuota 3)...")
	t0 := time.Now()
	tally := fire(30, func(i int) string {
		return call("POST", fmt.Sprintf("/supervisors/%d/assign", sid), admin.Token,
			map[string]int{"student_id": thesisStud[i]}).label()
	})
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))
	ok := printSupervisor("Dosen Rebutan")
	verdict(ok && tally["201 OK"] == 3 && tally["409 SUPERVISOR_FULL"] == 27, "D.3")
}

// ================================================================
// D.2 butir 2: satu mahasiswa ditetapkan ke dua pembimbing serentak
// ================================================================

func modeDobelPembimbing() {
	sA, sB := supID["Dosen Pembimbing A"], supID["Dosen Pembimbing B"]
	_, _ = db.Exec(ctx, "UPDATE supervisors SET quota = 30 WHERE id = ANY($1)", []int{sA, sB})
	fmt.Println("30 mahasiswa, masing-masing ditetapkan ke A dan ke B pada saat yang sama (60 request)...")
	t0 := time.Now()
	tally := fire(60, func(i int) string {
		target := sA
		if i%2 == 1 {
			target = sB
		}
		return call("POST", fmt.Sprintf("/supervisors/%d/assign", target), admin.Token,
			map[string]int{"student_id": thesisStud[i/2]}).label()
	})
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))

	var dobel int
	_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT student_id FROM supervisions
		GROUP BY student_id HAVING COUNT(*) > 1) x`).Scan(&dobel)
	fmt.Printf("mahasiswa dengan lebih dari satu pembimbing: %d\n", dobel)
	okA := printSupervisor("Dosen Pembimbing A")
	okB := printSupervisor("Dosen Pembimbing B")
	verdict(okA && okB && dobel == 0 && tally["201 OK"] == 30 && tally["409 ALREADY_SUPERVISED"] == 30, "D.2 butir 2")
}

// ================================================================
// D.2 butir 3: pergantian berlawanan arah A <-> B
// ================================================================

func seedSupervision(name string, ids []int) {
	for _, s := range ids {
		_, _ = db.Exec(ctx, "INSERT INTO supervisions (student_id, supervisor_id) VALUES ($1, $2)", s, supID[name])
	}
	_, _ = db.Exec(ctx, `UPDATE supervisors s SET supervised_count =
		(SELECT COUNT(*) FROM supervisions v WHERE v.supervisor_id = s.id) WHERE s.name = $1`, name)
}

func modeGantiBalik() {
	sA, sB := supID["Dosen Pembimbing A"], supID["Dosen Pembimbing B"]
	// Kuota 30 supaya tidak ada yang penuh walau seluruh pergantian ke
	// satu arah kebetulan selesai lebih dulu; yang diuji di sini deadlock.
	_, _ = db.Exec(ctx, "UPDATE supervisors SET quota = 30 WHERE id = ANY($1)", []int{sA, sB})
	seedSupervision("Dosen Pembimbing A", thesisStud[0:15])
	seedSupervision("Dosen Pembimbing B", thesisStud[15:30])
	time.Sleep(1500 * time.Millisecond)
	before := deadlockCount()

	fmt.Println("15 pasang mahasiswa ganti pembimbing berlawanan arah (A ke B dan B ke A) serentak...")
	t0 := time.Now()
	tally := fire(30, func(i int) string {
		to := sB
		if i >= 15 {
			to = sA
		}
		return call("POST", "/supervisions/switch", admin.Token,
			map[string]int{"student_id": thesisStud[i], "to_supervisor_id": to}).label()
	})
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))
	time.Sleep(1500 * time.Millisecond)
	dl := deadlockCount() - before
	fmt.Printf("deadlock terdeteksi PostgreSQL selama uji: %d\n", dl)
	okA := printSupervisor("Dosen Pembimbing A")
	okB := printSupervisor("Dosen Pembimbing B")
	verdict(okA && okB && dl == 0 && tally["200 OK"] == 30, "D.2 butir 3")
}

// ================================================================
// D.4 nomor 1: dua pergantian serentak untuk mahasiswa yang SAMA
// ================================================================

func modeGantiGanda() {
	sB, sR := supID["Dosen Pembimbing B"], supID["Dosen Rebutan"]
	_, _ = db.Exec(ctx, "UPDATE supervisors SET quota = 20 WHERE id = $1", sR)
	seedSupervision("Dosen Pembimbing A", thesisStud[0:10])

	fmt.Println("10 mahasiswa di A, masing-masing diganti ke B dan ke Dosen Rebutan pada saat yang sama (20 request)...")
	t0 := time.Now()
	tally := fire(20, func(i int) string {
		to := sB
		if i%2 == 1 {
			to = sR
		}
		return call("POST", "/supervisions/switch", admin.Token,
			map[string]int{"student_id": thesisStud[i/2], "to_supervisor_id": to}).label()
	})
	elapsed := time.Since(t0)
	printTally(tally)
	fmt.Printf("waktu total : %v\n", elapsed.Round(time.Millisecond))
	okA := printSupervisor("Dosen Pembimbing A")
	okB := printSupervisor("Dosen Pembimbing B")
	okR := printSupervisor("Dosen Rebutan")
	var total int
	_ = db.QueryRow(ctx, "SELECT COUNT(*) FROM supervisions WHERE student_id = ANY($1)", thesisStud[0:10]).Scan(&total)
	fmt.Printf("baris supervisions milik 10 mahasiswa itu: %d\n", total)
	verdict(okA && okB && okR && total == 10 && tally["200 OK"] == 20, "D.4 nomor 1")
}
