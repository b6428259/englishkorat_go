# Schedules

Protected group `/api/schedules`

- POST / — create schedule (owner/admin)
  Body includes: course_id, assigned_to_teacher_id, room_id, schedule_name, schedule_type,
  recurring_pattern, total_hours, hours_per_session, session_per_week, max_students,
  start_date, estimated_end_date, auto_reschedule, notes, user_in_course_ids[], session_start_time,
  custom_recurring_days[]

- GET / — list all schedules (owner/admin) with filters: status, type, branch_id
- GET /my — schedules for current user (teacher/admin/owner by assigned; student by enrollment)
- PATCH /:id/confirm — assigned user confirms schedule (sets status)

Sessions
- GET /:id/sessions — list sessions for a schedule
- PATCH /sessions/:id/status — update session status (teacher must be assigned)
- POST /sessions/makeup — create a makeup session (class type)

Comments
- POST /comments — add a comment to schedule or session
- GET /comments?schedule_id=...&session_id=... — list comments
