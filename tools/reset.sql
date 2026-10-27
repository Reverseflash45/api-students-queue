-- Kembalikan keadaan sebelum mengulang uji beban.
--   psql -U postgres -d praktikum_backend -f tools/reset.sql
-- Alat uji beban juga menjalankan isi yang sama lewat flag -reset.

DELETE FROM enrollments
WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'mhs%');

UPDATE courses c
SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id);

DELETE FROM supervisions
WHERE student_id IN (SELECT id FROM students WHERE nim LIKE '999%');

UPDATE supervisors s
SET supervised_count = (SELECT COUNT(*) FROM supervisions v WHERE v.supervisor_id = s.id);
