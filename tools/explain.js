// Bukti index dipakai, untuk dijalankan di mongosh:
//   mongosh praktikum_backend tools/explain.js
// Jalankan setelah go run ./tools/uji10 -mode explain supaya ada datanya.

function ringkas(judul, hasil) {
  const s = hasil.executionStats;
  let stage = hasil.queryPlanner.winningPlan;
  let index = "";
  while (stage) {
    if (stage.indexName) { index = stage.indexName; break; }
    stage = stage.inputStage || (stage.queryPlan && stage.queryPlan.inputStage) || stage.queryPlan;
  }
  print("== " + judul + " ==");
  print("  index dipakai     : " + index);
  print("  totalKeysExamined : " + s.totalKeysExamined);
  print("  totalDocsExamined : " + s.totalDocsExamined);
  print("  nReturned         : " + s.nReturned);
}

const satu = db.student_portfolios.findOne({}, { student_id: 1 });
if (satu) {
  ringkas("portofolio satu mahasiswa",
    db.student_portfolios.find({ student_id: satu.student_id }).explain("executionStats"));
}
ringkas("cari portofolio berteknologi go",
  db.student_portfolios.find({ "projects.technologies": "go" }).sort({ student_id: 1 }).explain("executionStats"));
ringkas("daftar ulasan satu kelas",
  db.course_reviews.find({ course_id: 1 }).sort({ _id: -1 }).limit(11).explain("executionStats"));
ringkas("ulasan satu kelas dengan tag praktikal",
  db.course_reviews.find({ course_id: 1, tags: "praktikal" }).sort({ _id: -1 }).limit(11).explain("executionStats"));
