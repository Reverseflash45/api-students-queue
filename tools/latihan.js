// Latihan mongosh (Modul 10, Langkah 2)
//   mongosh tools/latihan.js
db = db.getSiblingDB("latihan_mongo");
db.mahasiswa.drop();
print("== insertOne ==");
printjson(db.mahasiswa.insertOne({ nim: "434221001", nama: "Andi", angkatan: 2023, minat: ["backend", "data"] }));
print("== insertMany ==");
printjson(db.mahasiswa.insertMany([
  { nim: "434221002", nama: "Budi", angkatan: 2023, minat: ["mobile"] },
  { nim: "434221003", nama: "Citra", angkatan: 2024, minat: ["data", "ai"], organisasi: { nama: "HIMA", jabatan: "Ketua" } }
]));
print("== find angkatan 2023 ==");
printjson(db.mahasiswa.find({ angkatan: 2023 }, { _id: 0, nama: 1, minat: 1 }).toArray());
print("== find minat data ==");
printjson(db.mahasiswa.find({ minat: "data" }, { _id: 0, nama: 1 }).toArray());
print("== find field bersarang ==");
printjson(db.mahasiswa.find({ "organisasi.jabatan": "Ketua" }, { _id: 0, nama: 1 }).toArray());
print("== updateOne $push ==");
printjson(db.mahasiswa.updateOne({ nim: "434221002" }, { $push: { minat: "backend" } }));
print("== updateOne $inc pada field yang belum ada ==");
printjson(db.mahasiswa.updateOne({ nim: "434221001" }, { $inc: { poin: 10 } }));
printjson(db.mahasiswa.findOne({ nim: "434221001" }, { _id: 0, nama: 1, poin: 1 }));
print("== createIndex unik ==");
printjson(db.mahasiswa.createIndex({ nim: 1 }, { unique: true }));
print("== insert nim ganda ==");
try { db.mahasiswa.insertOne({ nim: "434221001", nama: "Andi Palsu" }); } catch (e) { print(e.message); }
print("== deleteOne ==");
printjson(db.mahasiswa.deleteOne({ nim: "434221003" }));
print("== countDocuments ==");
print(db.mahasiswa.countDocuments());
print("== ObjectId berisi waktu ==");
const d = db.mahasiswa.findOne({ nim: "434221001" });
print(d._id); print(d._id.getTimestamp());
