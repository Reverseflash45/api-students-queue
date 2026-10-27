package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

var (
	ErrPortfolioNotFound = errors.New("portofolio belum dibuat")
	ErrProjectNotFound   = errors.New("proyek tidak ditemukan")
	ErrProjectLimit      = errors.New("jumlah proyek sudah mencapai batas")
)

// MaxProjectsPerPortfolio membatasi array projects, alasannya sama dengan
// batas balasan ulasan: satu dokumen maksimal 16 MB.
const MaxProjectsPerPortfolio = 20

const dateLayout = "2006-01-02"

type PortfolioRepository interface {
	EnsureIndexes(ctx context.Context) error
	FindByStudent(ctx context.Context, studentID int) (model.Portfolio, error)
	Upsert(ctx context.Context, p model.Portfolio) (model.Portfolio, bool, error)
	AddProject(ctx context.Context, studentID int, p model.Project) (model.Portfolio, error)
	RemoveProject(ctx context.Context, studentID int, projectID string) error
	FindByTechnology(ctx context.Context, tech string, limit int) ([]model.PortfolioBrief, error)
	TopTechnologies(ctx context.Context, limit int) ([]model.TechnologyCount, error)
}

// portfolioDocument adalah bentuk portofolio SEBAGAIMANA TERSIMPAN di MongoDB.
type portfolioDocument struct {
	ID           bson.ObjectID         `bson:"_id,omitempty"`
	StudentID    int                   `bson:"student_id"`
	Headline     string                `bson:"headline"`
	Bio          string                `bson:"bio"`
	Projects     []projectDocument     `bson:"projects"`
	Certificates []certificateDocument `bson:"certificates"`
	CreatedAt    time.Time             `bson:"created_at"`
	UpdatedAt    time.Time             `bson:"updated_at"`
}

type projectDocument struct {
	ID           bson.ObjectID `bson:"_id"`
	Title        string        `bson:"title"`
	Technologies []string      `bson:"technologies"`
	URL          string        `bson:"url,omitempty"`
	Year         int           `bson:"year"`
}

type certificateDocument struct {
	Name     string    `bson:"name"`
	Issuer   string    `bson:"issuer"`
	IssuedOn time.Time `bson:"issued_on"`
}

func toProjectDocument(p model.Project) projectDocument {
	techs := p.Technologies
	if techs == nil {
		techs = []string{}
	}
	return projectDocument{
		ID: bson.NewObjectID(), Title: p.Title, Technologies: techs, URL: p.URL, Year: p.Year,
	}
}

func (d portfolioDocument) toModel() model.Portfolio {
	projects := make([]model.Project, 0, len(d.Projects))
	for _, p := range d.Projects {
		projects = append(projects, model.Project{
			ID: p.ID.Hex(), Title: p.Title, Technologies: p.Technologies, URL: p.URL, Year: p.Year,
		})
	}
	certs := make([]model.Certificate, 0, len(d.Certificates))
	for _, c := range d.Certificates {
		certs = append(certs, model.Certificate{
			Name: c.Name, Issuer: c.Issuer, IssuedOn: c.IssuedOn.UTC().Format(dateLayout),
		})
	}
	return model.Portfolio{
		ID: d.ID.Hex(), StudentID: d.StudentID, Headline: d.Headline, Bio: d.Bio,
		Projects: projects, Certificates: certs, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type portfolioMongoRepository struct{ coll *mongo.Collection }

func NewPortfolioRepository(db *mongo.Database) PortfolioRepository {
	return &portfolioMongoRepository{coll: db.Collection("student_portfolios")}
}

func (r *portfolioMongoRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// Satu mahasiswa tepat SATU portofolio. Dijaga database, jadi
			// dua PUT pertama yang datang serentak tidak bisa membuat dua
			// dokumen untuk mahasiswa yang sama.
			Keys:    bson.D{{Key: "student_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("student_unique"),
		},
		{
			// Multikey index pada teknologi di dalam array projects, dipakai
			// GET /portfolios?technology=... Field student_id ikut di index
			// supaya pengurutan hasil tidak perlu sort di memori.
			Keys:    bson.D{{Key: "projects.technologies", Value: 1}, {Key: "student_id", Value: 1}},
			Options: options.Index().SetName("project_technologies"),
		},
	})
	if err != nil {
		return fmt.Errorf("membuat index student_portfolios: %w", err)
	}
	return nil
}

func (r *portfolioMongoRepository) FindByStudent(ctx context.Context, studentID int) (model.Portfolio, error) {
	var doc portfolioDocument
	if err := r.coll.FindOne(ctx, bson.M{"student_id": studentID}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Portfolio{}, ErrPortfolioNotFound
		}
		return model.Portfolio{}, fmt.Errorf("mengambil portofolio: %w", err)
	}
	return doc.toModel(), nil
}

// Upsert membuat portofolio baru atau mengganti seluruh isinya (PUT).
// Nilai kedua bernilai true bila dokumen baru dibuat.
//
// created_at dan _id hanya diisi saat dokumen pertama kali dibuat
// ($setOnInsert), jadi mengganti isi portofolio tidak mengubah keduanya.
func (r *portfolioMongoRepository) Upsert(ctx context.Context, p model.Portfolio) (model.Portfolio, bool, error) {
	t := now()
	projects := make([]projectDocument, 0, len(p.Projects))
	for _, pr := range p.Projects {
		projects = append(projects, toProjectDocument(pr))
	}
	certs := make([]certificateDocument, 0, len(p.Certificates))
	for _, c := range p.Certificates {
		// Format tanggal sudah dijamin validator (datetime=2006-01-02).
		issued, _ := time.Parse(dateLayout, c.IssuedOn)
		certs = append(certs, certificateDocument{Name: c.Name, Issuer: c.Issuer, IssuedOn: issued})
	}

	filter := bson.M{"student_id": p.StudentID}
	// bson.D, bukan bson.M, supaya urutan field di dokumen selalu sama
	// (map di Go tidak punya urutan) dan dokumen enak dibaca di Compass.
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "headline", Value: p.Headline},
			{Key: "bio", Value: p.Bio},
			{Key: "projects", Value: projects},
			{Key: "certificates", Value: certs},
			{Key: "updated_at", Value: t},
		}},
		{Key: "$setOnInsert", Value: bson.D{{Key: "created_at", Value: t}}},
	}

	res, err := r.coll.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil && mongo.IsDuplicateKeyError(err) {
		// Dua PUT pertama untuk mahasiswa yang sama datang bersamaan:
		// keduanya mencoba insert dan index unik menolak salah satunya.
		// Saat diulang, dokumennya sudah ada, jadi yang terjadi adalah update.
		res, err = r.coll.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	}
	if err != nil {
		return model.Portfolio{}, false, fmt.Errorf("menyimpan portofolio: %w", err)
	}

	saved, err := r.FindByStudent(ctx, p.StudentID)
	if err != nil {
		return model.Portfolio{}, false, err
	}
	return saved, res.UpsertedCount == 1, nil
}

// AddProject menambah satu proyek dengan $push, dibatasi 20 proyek.
//
// Pola yang sama dengan "replies.49": dokumen hanya cocok bila elemen ke-20
// (indeks 19) belum ada. Pemeriksaan batas dan penambahan terjadi dalam satu
// operasi, jadi permintaan serentak tidak bisa sama-sama lolos ketika tinggal
// satu tempat.
func (r *portfolioMongoRepository) AddProject(ctx context.Context, studentID int, p model.Project) (model.Portfolio, error) {
	limitField := "projects." + strconv.Itoa(MaxProjectsPerPortfolio-1)

	var updated portfolioDocument
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"student_id": studentID, limitField: bson.M{"$exists": false}},
		bson.M{
			"$push": bson.M{"projects": toProjectDocument(p)},
			"$set":  bson.M{"updated_at": now()},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if err == nil {
		return updated.toModel(), nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.Portfolio{}, fmt.Errorf("menambah proyek: %w", err)
	}

	count, err := r.coll.CountDocuments(ctx, bson.M{"student_id": studentID})
	if err != nil {
		return model.Portfolio{}, fmt.Errorf("memeriksa portofolio: %w", err)
	}
	if count == 0 {
		return model.Portfolio{}, ErrPortfolioNotFound
	}
	return model.Portfolio{}, ErrProjectLimit
}

// RemoveProject menghapus satu proyek dengan $pull. Filter ikut memuat
// projects._id, jadi MatchedCount 0 berarti proyeknya tidak ada (atau
// portofolionya yang tidak ada), dan updated_at tidak ikut berubah.
func (r *portfolioMongoRepository) RemoveProject(ctx context.Context, studentID int, projectID string) error {
	oid, err := parseID(projectID)
	if err != nil {
		return err
	}

	res, err := r.coll.UpdateOne(ctx,
		bson.M{"student_id": studentID, "projects._id": oid},
		bson.M{
			"$pull": bson.M{"projects": bson.M{"_id": oid}},
			"$set":  bson.M{"updated_at": now()},
		},
	)
	if err != nil {
		return fmt.Errorf("menghapus proyek: %w", err)
	}
	if res.MatchedCount == 1 {
		return nil
	}

	count, err := r.coll.CountDocuments(ctx, bson.M{"student_id": studentID})
	if err != nil {
		return fmt.Errorf("memeriksa portofolio: %w", err)
	}
	if count == 0 {
		return ErrPortfolioNotFound
	}
	return ErrProjectNotFound
}

// FindByTechnology mencari portofolio yang punya minimal satu proyek dengan
// teknologi tertentu. tech selalu string, jadi tidak bisa berubah menjadi
// operator MongoDB.
func (r *portfolioMongoRepository) FindByTechnology(ctx context.Context, tech string, limit int) ([]model.PortfolioBrief, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "student_id", Value: 1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"student_id": 1, "headline": 1})
	cursor, err := r.coll.Find(ctx, bson.M{"projects.technologies": tech}, opts)
	if err != nil {
		return nil, fmt.Errorf("mencari portofolio: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []portfolioDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("membaca portofolio: %w", err)
	}
	result := make([]model.PortfolioBrief, 0, len(docs))
	for _, d := range docs {
		result = append(result, model.PortfolioBrief{StudentID: d.StudentID, Headline: d.Headline})
	}
	return result, nil
}

// TopTechnologies menghitung teknologi yang paling banyak dipakai di seluruh
// portofolio.
//
// $unwind pertama memecah satu portofolio menjadi satu dokumen per proyek,
// $unwind kedua memecah satu proyek menjadi satu dokumen per teknologi.
// Setelah itu $group bisa menghitung berapa proyek yang memakai setiap
// teknologi, dan $addToSet mencatat mahasiswa mana saja yang memakainya.
func (r *portfolioMongoRepository) TopTechnologies(ctx context.Context, limit int) ([]model.TechnologyCount, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$unwind", Value: "$projects"}},
		{{Key: "$unwind", Value: "$projects.technologies"}},
		{{Key: "$group", Value: bson.M{
			"_id":      "$projects.technologies",
			"projects": bson.M{"$sum": 1},
			"students": bson.M{"$addToSet": "$student_id"},
		}}},
		{{Key: "$project", Value: bson.M{
			"projects": 1,
			"students": bson.M{"$size": "$students"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "projects", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("menghitung teknologi: %w", err)
	}
	defer cursor.Close(ctx)

	var rows []struct {
		Tech     string `bson:"_id"`
		Projects int    `bson:"projects"`
		Students int    `bson:"students"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("membaca statistik teknologi: %w", err)
	}

	result := make([]model.TechnologyCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.TechnologyCount{
			Technology: row.Tech, ProjectCount: row.Projects, StudentCount: row.Students,
		})
	}
	return result, nil
}
