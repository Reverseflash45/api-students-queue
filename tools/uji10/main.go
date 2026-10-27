// Alat uji Modul 10 (ulasan kelas dan portofolio di MongoDB).
//
// Server harus menyala di terminal lain. Jalankan dari folder proyek supaya
// .env terbaca:
//
//	go run ./tools/uji10 -mode ulasan       uji Langkah 9, satu per satu
//	go run ./tools/uji10 -mode serentak     uji serentak: helpful, balasan, batas 20 proyek
//	go run ./tools/uji10 -mode portofolio   transkrip request dan response setiap endpoint C.2
//	go run ./tools/uji10 -mode injeksi      bukti NoSQL injection ditolak
//	go run ./tools/uji10 -mode explain      bukti index dipakai (executionStats)
//	go run ./tools/uji10 -mode contoh       isi satu portofolio lengkap untuk dilihat di Compass
//
// Setiap mode membersihkan data ujinya sendiri lebih dulu: ulasan milik akun
// mhs001 sampai mhs060 dan admin_uji, serta portofolio mahasiswa uji (NIM 999...).
// Portofolio contoh milik NIM 230001001 tidak ikut dihapus.
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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/config"
	"github.com/Reverseflash45/api-students-queue/database"
	"github.com/Reverseflash45/api-students-queue/helper"
)

var (
	base   string
	ctx    = context.Background()
	client = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 100}}
	pg     *pgxpool.Pool
	mdb    *mongo.Database

	mhs      []account // mhs001 .. mhs060
	admin    account
	courseID = map[string]int{}
	testStud []int // id students dengan NIM 999xxxxxx
	rafiID   int   // id students NIM 230001001
)

type account struct {
	ID       int
	Username string
	Token    string
}

func main() {
	mode := flag.String("mode", "ulasan", "ulasan | serentak | portofolio | injeksi | explain | contoh")
	flag.StringVar(&base, "base", "http://localhost:3000/api/v1", "alamat dasar API")
	flag.Parse()

	config.LoadEnv()
	var err error
	if pg, err = database.NewPool(ctx); err != nil {
		fmt.Println("gagal terhubung ke PostgreSQL:", err)
		os.Exit(1)
	}
	defer pg.Close()
	mc, err := database.NewMongoClient(ctx)
	if err != nil {
		fmt.Println("gagal terhubung ke MongoDB:", err)
		os.Exit(1)
	}
	defer mc.Disconnect(ctx)
	mdb = mc.Database(config.GetEnv("MONGO_DATABASE", "praktikum_backend"))

	if err := prepare(); err != nil {
		fmt.Println("persiapan gagal:", err)
		os.Exit(1)
	}
	cleanup()

	switch *mode {
	case "ulasan":
		modeUlasan()
	case "serentak":
		modeSerentak()
	case "portofolio":
		modePortofolio()
	case "injeksi":
		modeInjeksi()
	case "explain":
		modeExplain()
	case "contoh":
		modeContoh()
	default:
		fmt.Println("mode tidak dikenal:", *mode)
		os.Exit(1)
	}
}

// ================================================================
// Persiapan dan pembersihan
// ================================================================

func prepare() error {
	secret := config.GetEnv("JWT_SECRET", "")
	if len(secret) < 32 {
		return fmt.Errorf("JWT_SECRET di .env kosong atau terlalu pendek")
	}
	// Token dicetak sendiri karena alat ini memegang JWT_SECRET, sama seperti
	// alat uji beban Modul 9.
	jm := helper.NewJWTManager(secret, config.GetEnv("JWT_ISSUER", "praktikum-backend"), time.Hour)

	rows, err := pg.Query(ctx,
		"SELECT id, username, role FROM users WHERE username LIKE 'mhs%' OR username = 'admin_uji' ORDER BY username")
	if err != nil {
		return err
	}
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			return err
		}
		tok, err := jm.GenerateAccess(u)
		if err != nil {
			return err
		}
		a := account{ID: u.ID, Username: u.Username, Token: tok}
		if u.Username == "admin_uji" {
			admin = a
		} else {
			mhs = append(mhs, a)
		}
	}
	rows.Close()
	if len(mhs) < 60 || admin.ID == 0 {
		return fmt.Errorf("akun uji belum ada, jalankan migrations/007_seed_loadtest.sql")
	}

	rows, err = pg.Query(ctx, "SELECT code, id FROM courses")
	if err != nil {
		return err
	}
	for rows.Next() {
		var k string
		var v int
		_ = rows.Scan(&k, &v)
		courseID[k] = v
	}
	rows.Close()

	rows, err = pg.Query(ctx, "SELECT id FROM students WHERE nim LIKE '999%' ORDER BY nim")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		testStud = append(testStud, id)
	}
	rows.Close()
	if len(testStud) < 3 {
		return fmt.Errorf("mahasiswa uji belum ada, jalankan migrations/008_supervisors.sql")
	}
	_ = pg.QueryRow(ctx, "SELECT id FROM students WHERE nim = '230001001'").Scan(&rafiID)
	return nil
}

func cleanup() {
	ids := []int{admin.ID}
	for _, a := range mhs {
		ids = append(ids, a.ID)
	}
	_, _ = mdb.Collection("course_reviews").DeleteMany(ctx, bson.M{"user_id": bson.M{"$in": ids}})
	studs := testStud
	_, _ = mdb.Collection("student_portfolios").DeleteMany(ctx, bson.M{"student_id": bson.M{"$in": studs}})

	// KRS: mhs001 dan mhs002 terdaftar di IF101, mhs003 tidak.
	_, _ = pg.Exec(ctx, "DELETE FROM enrollments WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'mhs%')")
	for _, a := range mhs[:2] {
		_, _ = pg.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2)", courseID["IF101"], a.ID)
	}
	_, _ = pg.Exec(ctx, "UPDATE courses c SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id)")
}

// ================================================================
// HTTP
// ================================================================

type reply struct {
	Status   int
	Code     string
	Body     []byte
	Location string
}

func call(method, path, token string, body any) reply {
	var rdr io.Reader
	if body != nil {
		switch b := body.(type) {
		case string: // body mentah, dipakai uji injeksi
			rdr = strings.NewReader(b)
		default:
			raw, _ := json.Marshal(b)
			rdr = bytes.NewReader(raw)
		}
	}
	req, _ := http.NewRequest(method, base+path, rdr)
	if method != http.MethodGet && method != http.MethodDelete {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return reply{Code: "TIDAK_ADA_RESPONS"}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(b, &e)
	return reply{Status: resp.StatusCode, Code: e.Code, Body: b, Location: resp.Header.Get("Location")}
}

func (r reply) label() string {
	if r.Status >= 200 && r.Status < 300 {
		return fmt.Sprintf("%d OK", r.Status)
	}
	return fmt.Sprintf("%d %s", r.Status, r.Code)
}

// field membaca satu nilai dari response, misalnya "data.id".
func (r reply) field(path string) any {
	var v any
	_ = json.Unmarshal(r.Body, &v)
	for _, k := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func (r reply) str(path string) string {
	if v, ok := r.field(path).(string); ok {
		return v
	}
	return ""
}

func (r reply) num(path string) int {
	if v, ok := r.field(path).(float64); ok {
		return int(v)
	}
	return -1
}

// fire sama dengan Modul 9: semua goroutine berangkat bersamaan.
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
	close(start)
	wg.Wait()
	close(results)
	tally := map[string]int{}
	for r := range results {
		tally[r]++
	}
	return tally
}

func tallyText(t map[string]int) string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d x %s", t[k], k))
	}
	return strings.Join(parts, ", ")
}

var pass, fail int

func check(name string, ok bool, detail string) {
	if ok {
		pass++
		fmt.Printf("[OK]    %-55s %s\n", name, detail)
	} else {
		fail++
		fmt.Printf("[SALAH] %-55s %s\n", name, detail)
	}
}

func summary() {
	fmt.Printf("\nRINGKASAN: %d uji, %d sesuai, %d salah\n", pass+fail, pass, fail)
}

// ================================================================
// Mode ulasan: tabel Langkah 9
// ================================================================

func modeUlasan() {
	m1, m2, m3 := mhs[0], mhs[1], mhs[2]
	c := courseID["IF101"]
	fmt.Println("mhs001 dan mhs002 terdaftar di IF101; mhs003 tidak.")

	r := call("POST", fmt.Sprintf("/courses/%d/reviews", c), m3.Token,
		map[string]any{"rating": 4, "comment": "Kelasnya seru sekali", "tags": []string{"seru"}})
	check("mhs003 menulis ulasan IF101", r.Status == 403 && r.Code == "NOT_ENROLLED", r.label())

	r = call("POST", fmt.Sprintf("/courses/%d/reviews", c), m1.Token, map[string]any{
		"rating": 5, "comment": "Materi Go-nya jelas, tugasnya cukup banyak",
		"tags": []string{"Tugas-Banyak", " dosen-jelas ", "tugas-banyak"}})
	tags, _ := json.Marshal(r.field("data.tags"))
	id1 := r.str("data.id")
	check("mhs001 menulis ulasan, tag dinormalisasi", r.Status == 201 && string(tags) == `["tugas-banyak","dosen-jelas"]` && r.Location != "",
		fmt.Sprintf("%s, tags %s, Location %s", r.label(), tags, r.Location))

	r = call("POST", fmt.Sprintf("/courses/%d/reviews", c), m1.Token,
		map[string]any{"rating": 3, "comment": "Ulasan kedua saya untuk kelas ini"})
	check("mhs001 menulis ulasan kedua di kelas yang sama", r.Status == 409 && r.Code == "ALREADY_REVIEWED", r.label())

	r = call("POST", fmt.Sprintf("/courses/%d/reviews", c), m2.Token, map[string]any{
		"rating": 9, "comment": "pendek", "tags": []string{"aa", "bb", "cc", "dd", "ee", "ff"}})
	fields, _ := json.Marshal(r.field("fields"))
	check("Rating 9, komentar pendek, 6 tag", r.Status == 422, fmt.Sprintf("%s %s", r.label(), fields))

	r = call("POST", "/courses/99999/reviews", m1.Token,
		map[string]any{"rating": 4, "comment": "Kelas yang tidak pernah ada"})
	check("Ulasan untuk kelas 99999", r.Status == 404, r.label())

	r = call("GET", "/reviews/bukan-objectid", m1.Token, nil)
	check("GET /reviews/bukan-objectid", r.Status == 400, r.label()+" \""+r.str("message")+"\"")

	r = call("PATCH", "/reviews/"+id1, m2.Token, map[string]any{"rating": 1})
	check("mhs002 mengubah ulasan mhs001", r.Status == 403, r.label())

	before := call("GET", "/reviews/"+id1, m1.Token, nil)
	time.Sleep(20 * time.Millisecond)
	r = call("PATCH", "/reviews/"+id1, m1.Token, map[string]any{"rating": 4})
	sameCreated := r.str("data.created_at") == before.str("data.created_at")
	changedUpdated := r.str("data.updated_at") != before.str("data.updated_at")
	check("mhs001 mengubah ulasannya sendiri", r.Status == 200 && sameCreated && changedUpdated,
		fmt.Sprintf("%s, created_at tetap=%v, updated_at berubah=%v", r.label(), sameCreated, changedUpdated))

	r = call("POST", "/reviews/"+id1+"/helpful", m1.Token, nil)
	check("mhs001 menilai ulasannya sendiri membantu", r.Status == 409 && r.Code == "OWN_REVIEW", r.label())

	r = call("POST", "/reviews/"+id1+"/helpful", m2.Token, nil)
	r2 := call("POST", "/reviews/"+id1+"/helpful", m2.Token, nil)
	check("mhs002 menilai membantu, lalu mengulanginya", r.Status == 200 && r.num("data.helpful_count") == 1 && r2.Code == "ALREADY_VOTED",
		fmt.Sprintf("%s (helpful_count %d), lalu %s", r.label(), r.num("data.helpful_count"), r2.label()))

	r = call("POST", fmt.Sprintf("/courses/%d/reviews", c), m2.Token,
		map[string]any{"rating": 4, "comment": "Praktikumnya membantu sekali", "tags": []string{"praktikal"}})
	id2 := r.str("data.id")

	r = call("DELETE", "/reviews/"+id2, m3.Token, nil)
	check("mhs003 menghapus ulasan mhs002", r.Status == 403, r.label())
	r = call("DELETE", "/reviews/"+id2, admin.Token, nil)
	check("admin menghapus ulasan mhs002", r.Status == 204, r.label())
	r = call("DELETE", "/reviews/"+id2, admin.Token, nil)
	check("Menghapus ulasan yang sudah terhapus", r.Status == 404, r.label())

	r = call("PATCH", "/reviews/"+id1, m1.Token, `{"comment": {"$ne": ""}}`)
	check(`Body {"comment": {"$ne": ""}}`, r.Status == 400, r.label()+" \""+r.str("message")+"\"")

	r = call("GET", fmt.Sprintf(`/courses/%d/reviews?tag={"$ne":""}`, c), m1.Token, nil)
	list, _ := r.field("data").([]any)
	check(`?tag={"$ne":""} pada daftar ulasan`, r.Status == 200 && len(list) == 0,
		fmt.Sprintf("%s dengan %d hasil", r.label(), len(list)))

	r = call("GET", fmt.Sprintf("/courses/%d/reviews/summary", c), m1.Token, nil)
	sum, _ := json.Marshal(r.field("data"))
	check("Ringkasan ulasan IF101", r.Status == 200 && r.num("data.count") == 1, string(sum))

	summary()
}

// ================================================================
// Mode serentak
// ================================================================

func modeSerentak() {
	c := courseID["IF101"]
	m1, m2 := mhs[0], mhs[1]

	r := call("POST", fmt.Sprintf("/courses/%d/reviews", c), m1.Token,
		map[string]any{"rating": 5, "comment": "Materi Go-nya jelas, tugasnya cukup banyak"})
	id1 := r.str("data.id")
	r = call("POST", fmt.Sprintf("/courses/%d/reviews", c), m2.Token,
		map[string]any{"rating": 4, "comment": "Praktikumnya membantu sekali"})
	id2 := r.str("data.id")

	fmt.Println("== 1. satu mahasiswa (mhs002) menekan membantu 20 kali serentak ==")
	t := fire(20, func(int) string { return call("POST", "/reviews/"+id1+"/helpful", m2.Token, nil).label() })
	hc := call("GET", "/reviews/"+id1, m1.Token, nil).num("data.helpful_count")
	fmt.Printf("hasil: %s\nhelpful_count akhir: %d\n", tallyText(t), hc)
	check("1 berhasil, 19 ditolak, helpful_count 1", t["200 OK"] == 1 && t["409 ALREADY_VOTED"] == 19 && hc == 1, "")

	fmt.Println("\n== 2. 30 mahasiswa berbeda (mhs011 sampai mhs040) menilai serentak ==")
	t = fire(30, func(i int) string { return call("POST", "/reviews/"+id2+"/helpful", mhs[10+i].Token, nil).label() })
	hc = call("GET", "/reviews/"+id2, m1.Token, nil).num("data.helpful_count")
	fmt.Printf("hasil: %s\nhelpful_count akhir: %d\n", tallyText(t), hc)
	check("30 berhasil, helpful_count 30", t["200 OK"] == 30 && hc == 30, "")

	fmt.Println("\n== 3. ulasan berisi 48 balasan, lalu 10 balasan dikirim serentak ==")
	for i := 0; i < 48; i++ {
		call("POST", "/reviews/"+id1+"/replies", mhs[i%60].Token, map[string]string{"body": fmt.Sprintf("balasan ke-%d", i+1)})
	}
	t = fire(10, func(i int) string {
		return call("POST", "/reviews/"+id1+"/replies", mhs[50+i].Token, map[string]string{"body": "balasan serentak"}).label()
	})
	rv := call("GET", "/reviews/"+id1, m1.Token, nil)
	replies, _ := rv.field("data.replies").([]any)
	last := call("POST", "/reviews/"+id1+"/replies", m2.Token, map[string]string{"body": "balasan ke-51"})
	fmt.Printf("hasil: %s\nbalasan akhir: %d\nbalasan ke-51: %s\n", tallyText(t), len(replies), last.label())
	check("tepat 2 balasan masuk, total 50", t["201 OK"] == 2 && t["409 REPLY_LIMIT"] == 8 && len(replies) == 50, "")

	fmt.Println("\n== 4. portofolio berisi 18 proyek, lalu 10 proyek ditambah serentak ==")
	sid := testStud[0]
	projects := []map[string]any{}
	for i := 1; i <= 18; i++ {
		projects = append(projects, map[string]any{"title": fmt.Sprintf("Proyek Uji %02d", i), "technologies": []string{"go"}, "year": 2025})
	}
	r = call("PUT", fmt.Sprintf("/students/%d/portfolio", sid), admin.Token,
		map[string]any{"headline": "Portofolio uji batas proyek", "projects": projects})
	fmt.Printf("PUT portofolio 18 proyek: %s\n", r.label())
	t = fire(10, func(i int) string {
		return call("POST", fmt.Sprintf("/students/%d/portfolio/projects", sid), admin.Token,
			map[string]any{"title": fmt.Sprintf("Proyek Serentak %02d", i+1), "technologies": []string{"go", "mongodb"}, "year": 2026}).label()
	})
	var doc struct {
		Projects []bson.M `bson:"projects"`
	}
	_ = mdb.Collection("student_portfolios").FindOne(ctx, bson.M{"student_id": sid}).Decode(&doc)
	fmt.Printf("hasil: %s\njumlah proyek di MongoDB: %d\n", tallyText(t), len(doc.Projects))
	check("tepat 2 proyek masuk, total 20", t["201 OK"] == 2 && t["409 PROJECT_LIMIT"] == 8 && len(doc.Projects) == 20, "")

	summary()
}

// ================================================================
// Mode portofolio: transkrip setiap endpoint C.2
// ================================================================

func show(title, method, path string, tok account, body any, maxLines int) reply {
	fmt.Printf("\n### %s\n", title)
	fmt.Printf("> %s /api/v1%s   (token %s)\n", method, path, tok.Username)
	if body != nil {
		var raw []byte
		if s, ok := body.(string); ok {
			raw = []byte(s)
		} else {
			raw, _ = json.Marshal(body)
		}
		if len(raw) > 160 {
			raw = append(raw[:157], []byte("...")...)
		}
		fmt.Printf("> %s\n", raw)
	}
	r := call(method, path, tok.Token, body)
	fmt.Printf("< %d %s\n", r.Status, http.StatusText(r.Status))
	if len(r.Body) > 0 {
		var pretty bytes.Buffer
		if json.Indent(&pretty, r.Body, "", "  ") == nil {
			lines := strings.Split(pretty.String(), "\n")
			if len(lines) > maxLines {
				lines = append(lines[:maxLines], fmt.Sprintf("  ... (%d baris lagi)", len(lines)-maxLines))
			}
			fmt.Println(strings.Join(lines, "\n"))
		}
	}
	return r
}

func modePortofolio() {
	sid := testStud[1]
	p := fmt.Sprintf("/students/%d/portfolio", sid)

	show("1. GET portofolio yang belum dibuat (gagal)", "GET", p, admin, nil, 10)
	r := show("2. PUT membuat portofolio baru", "PUT", p, admin, map[string]any{
		"headline": "Backend developer, Go dan PostgreSQL",
		"bio":      "Mahasiswa D4 Teknik Informatika yang senang membangun API.",
		"projects": []map[string]any{
			{"title": "API Mahasiswa", "technologies": []string{"Go", "Fiber", "PostgreSQL"}, "url": "https://github.com/contoh/api", "year": 2026},
			{"title": "Dashboard Nilai", "technologies": []string{"Laravel", "MySQL"}, "year": 2025},
		},
		"certificates": []map[string]any{{"name": "Website Development: Back End", "issuer": "MySkill", "issued_on": "2025-08-01"}},
	}, 40)
	firstProject, _ := r.field("data.projects").([]any)
	show("3. PUT mengganti headline (portofolio sudah ada, jadi 200)", "PUT", p, admin, map[string]any{
		"headline": "Backend developer, Go, PostgreSQL, dan MongoDB",
		"bio":      "Mahasiswa D4 Teknik Informatika yang senang membangun API.",
		"projects": []map[string]any{
			{"title": "API Mahasiswa", "technologies": []string{"go", "fiber", "postgresql", "mongodb"}, "url": "https://github.com/contoh/api", "year": 2026},
		},
	}, 12)
	show("4. GET portofolio", "GET", p, mhs[0], nil, 30)
	r = show("5. POST menambah satu proyek", "POST", p+"/projects", admin,
		map[string]any{"title": "Analisis Sentimen Berita", "technologies": []string{"Python", "IndoBERT"}, "year": 2026}, 14)
	projects, _ := r.field("data.projects").([]any)
	newID := ""
	if len(projects) > 0 {
		newID, _ = projects[len(projects)-1].(map[string]any)["id"].(string)
	}
	show("6. DELETE proyek yang baru ditambah", "DELETE", p+"/projects/"+newID, admin, nil, 5)
	// Beberapa portofolio lain supaya statistik teknologi punya isi.
	others := [][]string{{"Go", "PostgreSQL"}, {"Laravel", "MySQL"}, {"Go", "MongoDB", "Docker"}, {"Flutter", "Dart"}, {"Python", "MySQL"}}
	for i, t := range others {
		call("PUT", fmt.Sprintf("/students/%d/portfolio", testStud[3+i]), admin.Token, map[string]any{
			"headline": fmt.Sprintf("Portofolio uji %02d", i+1),
			"projects": []map[string]any{{"title": "Proyek Utama", "technologies": t, "year": 2026}},
		})
	}
	fmt.Println("\n(5 portofolio lain dibuat lebih dulu supaya statistik punya isi)")
	show("7. GET statistik teknologi", "GET", "/portfolios/stats/technologies", mhs[0], nil, 40)
	show("8. GET cari portofolio yang memakai mongodb", "GET", "/portfolios?technology=mongodb", mhs[0], nil, 20)

	fmt.Println("\n================ kasus gagal ================")
	show("9. PUT untuk mahasiswa yang tidak ada di PostgreSQL", "PUT", "/students/99999/portfolio", admin,
		map[string]any{"headline": "Tidak akan tersimpan"}, 10)
	show("10. PUT dengan isi tidak valid", "PUT", p, admin, map[string]any{
		"headline":     "ab",
		"projects":     []map[string]any{{"title": "X", "technologies": []string{}, "url": "bukan-url", "year": 1990}},
		"certificates": []map[string]any{{"name": "Sertifikat", "issuer": "MySkill", "issued_on": "01-08-2025"}},
	}, 20)
	show("11. mhs001 (bukan pemilik data) menambah proyek", "POST", p+"/projects", mhs[0],
		map[string]any{"title": "Proyek Orang Lain", "technologies": []string{"go"}, "year": 2026}, 10)
	show("12. DELETE dengan id proyek yang bukan ObjectID", "DELETE", p+"/projects/123", admin, nil, 10)
	show("13. DELETE proyek yang sudah dihapus", "DELETE", p+"/projects/"+newID, admin, nil, 10)
	show("14. POST proyek ke mahasiswa yang belum punya portofolio", "POST",
		fmt.Sprintf("/students/%d/portfolio/projects", testStud[2]), admin,
		map[string]any{"title": "Proyek Pertama", "technologies": []string{"go"}, "year": 2026}, 10)
	_ = firstProject
}

// ================================================================
// Mode injeksi
// ================================================================

func modeInjeksi() {
	sid := testStud[1]
	p := fmt.Sprintf("/students/%d/portfolio", sid)
	call("PUT", p, admin.Token, map[string]any{"headline": "Portofolio uji injeksi",
		"projects": []map[string]any{{"title": "API Mahasiswa", "technologies": []string{"go"}, "year": 2026}}})

	show("1. operator di posisi string pada body PUT", "PUT", p, admin, `{"headline": {"$ne": ""}}`, 10)
	show("2. operator di dalam daftar teknologi", "POST", p+"/projects", admin,
		`{"title": "Proyek Injeksi", "technologies": [{"$gt": ""}], "year": 2026}`, 10)
	show("3. operator lewat query string pencarian", "GET", `/portfolios?technology={"$ne":""}`, mhs[0], nil, 10)
	show("4. pembanding: pencarian biasa", "GET", "/portfolios?technology=go", mhs[0], nil, 12)

	var n int64
	n, _ = mdb.Collection("student_portfolios").CountDocuments(ctx, bson.M{"student_id": sid, "headline": "Portofolio uji injeksi"})
	fmt.Printf("\nisi portofolio di MongoDB tidak berubah: %v\n", n == 1)
}

// ================================================================
// Mode explain
// ================================================================

// asM menyeragamkan dokumen hasil decode, yang bisa berbentuk bson.M atau bson.D.
func asM(v any) bson.M {
	switch t := v.(type) {
	case bson.M:
		return t
	case bson.D:
		m := bson.M{}
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return m
	}
	return bson.M{}
}

// winningIndex mencari nama index di dalam winningPlan, seberapa pun dalam.
func winningIndex(v any) string {
	switch t := v.(type) {
	case bson.M:
		if name, ok := t["indexName"].(string); ok {
			return name
		}
		for _, k := range []string{"inputStage", "queryPlan", "winningPlan"} {
			if s := winningIndex(t[k]); s != "" {
				return s
			}
		}
		if arr, ok := t["inputStages"].(bson.A); ok {
			for _, x := range arr {
				if s := winningIndex(x); s != "" {
					return s
				}
			}
		}
	case bson.D:
		m := bson.M{}
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return winningIndex(m)
	}
	return ""
}

func explain(title, coll string, filter bson.D, sortBy bson.D, limit int) {
	cmd := bson.D{{Key: "find", Value: coll}, {Key: "filter", Value: filter}}
	if sortBy != nil {
		cmd = append(cmd, bson.E{Key: "sort", Value: sortBy})
	}
	if limit > 0 {
		cmd = append(cmd, bson.E{Key: "limit", Value: limit})
	}
	var out bson.M
	err := mdb.RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Decode(&out)
	if err != nil {
		fmt.Println(title, "gagal:", err)
		return
	}
	qp := asM(out["queryPlanner"])
	es := asM(out["executionStats"])
	f, _ := bson.MarshalExtJSON(filter, false, false)
	fmt.Printf("\n%s\n  filter            : %s\n", title, f)
	fmt.Printf("  index dipakai     : %s\n", winningIndex(qp["winningPlan"]))
	fmt.Printf("  totalKeysExamined : %v\n", es["totalKeysExamined"])
	fmt.Printf("  totalDocsExamined : %v\n", es["totalDocsExamined"])
	fmt.Printf("  nReturned         : %v\n", es["nReturned"])
	if fmt.Sprint(es["totalDocsExamined"]) == fmt.Sprint(es["nReturned"]) {
		fmt.Println("  => dokumen diperiksa = dokumen dikembalikan, tidak ada yang dibaca sia-sia")
	}
}

func modeExplain() {
	// Data contoh: 8 portofolio, sebagian memakai go, dan 2 ulasan.
	techs := [][]string{{"go", "postgresql"}, {"laravel", "mysql"}, {"go", "mongodb"}, {"flutter", "dart"},
		{"python", "indobert"}, {"go", "fiber"}, {"react", "javascript"}, {"php", "mysql"}}
	for i, t := range techs {
		if i >= len(testStud) {
			break
		}
		call("PUT", fmt.Sprintf("/students/%d/portfolio", testStud[i]), admin.Token, map[string]any{
			"headline": fmt.Sprintf("Portofolio uji %02d", i+1),
			"projects": []map[string]any{{"title": "Proyek Utama", "technologies": t, "year": 2026}},
		})
	}
	c := courseID["IF101"]
	call("POST", fmt.Sprintf("/courses/%d/reviews", c), mhs[0].Token, map[string]any{"rating": 5, "comment": "Materi Go-nya jelas sekali", "tags": []string{"praktikal"}})
	call("POST", fmt.Sprintf("/courses/%d/reviews", c), mhs[1].Token, map[string]any{"rating": 4, "comment": "Tugasnya banyak tapi seru", "tags": []string{"seru"}})

	explain("1. portofolio satu mahasiswa (GET /students/:id/portfolio)", "student_portfolios",
		bson.D{{Key: "student_id", Value: testStud[0]}}, nil, 0)
	explain("2. cari portofolio berteknologi go (GET /portfolios?technology=go)", "student_portfolios",
		bson.D{{Key: "projects.technologies", Value: "go"}}, bson.D{{Key: "student_id", Value: 1}}, 21)
	explain("3. daftar ulasan satu kelas (GET /courses/:id/reviews)", "course_reviews",
		bson.D{{Key: "course_id", Value: c}}, bson.D{{Key: "_id", Value: -1}}, 11)
	explain("4. ulasan satu kelas dengan tag praktikal", "course_reviews",
		bson.D{{Key: "course_id", Value: c}, {Key: "tags", Value: "praktikal"}}, bson.D{{Key: "_id", Value: -1}}, 11)
}

// ================================================================
// Mode contoh: satu portofolio lengkap untuk dilihat di Compass
// ================================================================

func modeContoh() {
	if rafiID == 0 {
		fmt.Println("mahasiswa NIM 230001001 tidak ditemukan")
		return
	}
	p := fmt.Sprintf("/students/%d/portfolio", rafiID)
	show("Portofolio lengkap", "PUT", p, admin, map[string]any{
		"headline": "Mahasiswa D4 Teknik Informatika, backend dan machine learning",
		"bio":      "Senang membangun REST API dengan Go dan Laravel, serta mengolah teks berbahasa Indonesia dengan IndoBERT.",
		"projects": []map[string]any{
			{"title": "Analisis Sentimen Berita Emiten", "technologies": []string{"Python", "IndoBERT", "Pandas"}, "url": "https://github.com/Reverseflash45/sentimen-emiten", "year": 2026},
			{"title": "REST API Mahasiswa dengan Transaction", "technologies": []string{"Go", "Fiber", "PostgreSQL"}, "url": "https://github.com/Reverseflash45/api-students-tx", "year": 2026},
			{"title": "Sistem Rumah Sakit Hewan Pendidikan", "technologies": []string{"Laravel", "PHP", "MySQL"}, "year": 2025},
			{"title": "Aplikasi Tracking Produktivitas", "technologies": []string{"Flutter", "Dart", "Supabase"}, "year": 2025},
		},
		"certificates": []map[string]any{
			{"name": "Website Development: Back End", "issuer": "MySkill", "issued_on": "2025-01-15"},
			{"name": "UX Writing", "issuer": "MySkill x Lion Parcel", "issued_on": "2025-03-10"},
		},
	}, 120)
	fmt.Printf("\nBuka Compass, database %s, collection student_portfolios, filter { student_id: %d }\n",
		config.GetEnv("MONGO_DATABASE", "praktikum_backend"), rafiID)
}
