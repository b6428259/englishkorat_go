# Schedules

Protected group `/api/schedules`

- POST / — create schedule (owner/admin)
  Body includes: course_id, assigned_to_teacher_id, room_id, schedule_name, schedule_type,
  recurring_pattern, total_hours, hours_per_session, session_per_week, max_students,
  start_date, estimated_end_date, auto_reschedule, notes, user_in_course_ids[], session_start_time,
  custom_recurring_days[]
  - See `SCHEDULE-CLASS-CASE.md` for class-specific scenarios and payload examples.
- POST /preview — dry-run validator (owner/admin). Generates the same sessions the create endpoint would and returns a readiness summary: room/teacher/participant conflicts, holiday impacts, group payment status (for class schedules), and whether the plan can be safely created. Works for every schedule type (`class`, `meeting`, `event`, `holiday`, `appointment`). For non-class types the preview still runs even if you omit `participant_user_ids`, but it will raise a warning because the final create call requires them.
- POST /rooms/check-conflicts — pre-check room availability for a proposed schedule (owner/admin). Accepts either a single `room_id` or `room_ids[]` to evaluate multiple rooms in one call. Applicable to all schedule types; simply pass the timing fields you intend to use.

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
