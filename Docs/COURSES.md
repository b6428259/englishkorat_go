# Courses

Public endpoints under `/api/public`
- GET /courses — list courses (supports branch_id, status, course_type, level)
- GET /courses/:id — course detail
- GET /courses/branch/:branch_id — active courses by branch

Protected (owner/admin) under `/api/courses`
- POST / — create course
- PUT /:id — update course
- DELETE /:id — delete (soft)

## Assignments

- POST /api/courses/:id/assignments — assign single user
Body:
{
  "user_id": 10,
  "role": "student",  // optional: instructor/assistant/observer/student/teacher
  "status": "enrolled" // optional
}

- POST /api/courses/:id/assignments/bulk — assign multiple users
Body (array):
[
  { "user_id": 10, "role": "student", "status": "enrolled" },
  { "user_id": 11 },
  { "user_id": 2, "role": "teacher" }
]

Response summary fields: processed, created, updated, unchanged, failed, results[]
