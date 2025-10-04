# Schedule Class Case Guide

This guide documents common class-scheduling scenarios handled by `POST /api/schedules` and shows how the backend validates and generates sessions in each case.

## Prerequisites

- Request must be authenticated as an `admin` or `owner`.
- `schedule_type` must be `class` and `group_id` is required.
- The selected group must have at least one member whose `payment_status` is `deposit_paid` or `fully_paid`.
- `total_hours` and `hours_per_session` must be positive integers where `total_hours % hours_per_session == 0`.
- Branch operating hours default to 08:00–21:00 unless overridden by the course's branch (`branch.open_time`, `branch.close_time`).

## Case 1 — Weekly Single-Slot Class (Legacy Flow)

Use when every session occurs on the same weekday/time. Provide `recurring_pattern` (e.g., `weekly`) and `session_start_time`.

```json
{
  "schedule_name": "G5 Grammar Prep",
  "schedule_type": "class",
  "group_id": 42,
  "recurring_pattern": "weekly",
  "total_hours": 24,
  "hours_per_session": 2,
  "session_per_week": 1,
  "start_date": "2025-10-06T00:00:00Z",
  "estimated_end_date": "2025-12-29T00:00:00Z",
  "session_start_time": "18:00",
  "default_teacher_id": 17,
  "default_room_id": 8,
  "auto_reschedule": false,
  "notes": "Evening grammar drills"
}
```

- Backend generates 12 weekly sessions (24 ÷ 2) starting on the provided `start_date` weekday.
- Branch hours are validated against the single `session_start_time`.

## Case 2 — Multi-Slot Weekly Class (session_times)

Use when the class meets multiple days in the same week. Define `session_times` payload entries in place of `session_start_time`.

```json
{
  "schedule_name": "IELTS Intensive",
  "schedule_type": "class",
  "group_id": 51,
  "recurring_pattern": "custom",
  "total_hours": 36,
  "hours_per_session": 2,
  "session_per_week": 3,
  "start_date": "2025-10-07T00:00:00Z",
  "estimated_end_date": "2025-11-18T00:00:00Z",
  "session_times": [
    { "weekday": 1, "start_time": "09:00" },
    { "weekday": 3, "start_time": "09:00" },
    { "weekday": 5, "start_time": "09:00" }
  ],
  "default_teacher_id": 23,
  "default_room_id": 4,
  "auto_reschedule": true
}
```

- `session_per_week` **must** equal the number of entries in `session_times`.
- Weekday uses `0` (Sunday) through `6` (Saturday).
- Duplicate weekdays are rejected.
- Sessions are generated per slot; 18 sessions in total (36 ÷ 2).
- `recurring_pattern` is forced to `custom` when `session_times` are supplied.

## Case 3 — Branch Operating-Hour Enforcement

The backend checks that each slot is within the branch's operating window. If a course branch has `open_time = "10:00:00"` and `close_time = "20:00:00"`, any slot outside 10:00–20:00 is rejected.

Example failure response:

```json
{
  "error": "session on weekday 6 (21:00) is outside branch operating hours"
}
```

If the course has no branch or times are missing, defaults (08:00–21:00) apply.

## Case 4 — Holiday Auto-Rescheduling

When `auto_reschedule` is `true`, the service fetches Thai government holidays for the span covering:

- Schedule `start_date`
- Generated sessions' min/max dates

Sessions landing on a holiday are deferred to the next non-holiday day, preserving weekday ordering, then reindexed.

### Example

- Holiday: 2025-12-05 (King Bhumibol Memorial Day)
- Original session: Friday 2025-12-05 09:00–11:00
- Result: moved to Saturday 2025-12-06 09:00–11:00, `week_number` recalculated after adjustments.

## Case 5 — Conflict Detection Before Persisting

All generated sessions run through conflict checks before any database write:

- **Group conflict**: Prevents multiple active class schedules for the same group.
- **Teacher conflict**: Checks existing sessions for default teacher (`assigned` or `scheduled`) within the same date range.
- **Room conflict**: Ensures no overlapping sessions using the same room (session-level or schedule default).
- **Participant conflict**: Applies only to non-class schedules but noted for completeness.

Response example when teacher is double-booked:

```json
{
  "error": "teacher JaneDoe already has a conflicting session at 09:00-11:00"
}
```

## Case 6 — Rolling Session End-Date Adjustment

After generation (and optional rescheduling) the final `estimated_end_date` is set to the last session's date. If the request supplied an earlier value, it will be updated automatically.

## Response Structure

Successful creation returns the persisted schedule with preloaded relations:

```json
{
  "message": "Schedule created successfully",
  "schedule": {
    "id": 1287,
    "schedule_name": "IELTS Intensive",
    "schedule_type": "class",
    "group_id": 51,
    "status": "assigned",
    "auto_reschedule_holiday": true,
    "start_date": "2025-10-07T00:00:00Z",
    "estimated_end_date": "2025-11-18T00:00:00Z",
    "default_teacher": { "id": 23, "username": "janedoe" },
    "default_room": { "id": 4, "name": "Room 204" }
  }
}
```

## API — Room Conflict Pre-check

Use `POST /api/schedules/rooms/check-conflicts` to verify room availability before creating a schedule. This endpoint accepts the same timing payload as schedule creation and returns any overlapping sessions. You can provide either a single `room_id` (legacy clients) or a `room_ids` array to pre-check several rooms in one call. For non-class schedule types (meeting, event, appointment, holiday) the payload shape is identical—just omit `group_id` and pass whichever timing fields you plan to persist.

### Request Example (Multi-slot)

```json
{
  "room_ids": [4, 7, 9],
  "branch_id": 2,
  "recurring_pattern": "custom",
  "total_hours": 36,
  "hours_per_session": 2,
  "session_per_week": 3,
  "start_date": "2025-10-07T00:00:00Z",
  "estimated_end_date": "2025-11-18T00:00:00Z",
  "session_times": [
    { "weekday": 1, "start_time": "09:00" },
    { "weekday": 3, "start_time": "09:00" },
    { "weekday": 5, "start_time": "09:00" }
  ]
}
```

### Response Example

```json
{
  "has_conflict": true,
  "checked_room_ids": [4, 7, 9],
  "conflicts": [
    {
      "room_id": 4,
      "existing_room_id": 4,
      "session_id": 9123,
      "schedule_id": 440,
      "schedule_name": "IELTS Intensive",
      "session_date": "2025-10-14",
      "start_time": "09:00",
      "end_time": "11:00"
    }
  ],
  "rooms": [
    {
      "room_id": 4,
      "conflicts": [
        {
          "room_id": 4,
          "existing_room_id": 4,
          "session_id": 9123,
          "schedule_id": 440,
          "schedule_name": "IELTS Intensive",
          "session_date": "2025-10-14",
          "start_time": "09:00",
          "end_time": "11:00"
        }
      ]
    },
    { "room_id": 7, "conflicts": [] },
    { "room_id": 9, "conflicts": [] }
  ]
}
```

To ignore conflicts from an existing schedule (e.g., during updates), include `"exclude_schedule_id": 440`.

## API — Schedule Preview (Dry Run)

Before final submission, call `POST /api/schedules/preview` with the exact payload you intend to send to `POST /api/schedules`. The preview endpoint generates sessions, applies holiday rescheduling, and returns a comprehensive readiness report:

- `can_create`: `true` when no blocking issues are detected.
- `issues`: ordered list of `error` or `warning` findings (room/teacher/participant conflicts, group payment status, holiday overlap, input validation).
- `sessions` / `original_sessions`: generated sessions after and before holiday rescheduling.
- `holiday_impacts`: any sessions that hit Thai public holidays and where they would be moved.
- `conflicts`: detailed breakdown for rooms, teachers, participants, and (for class schedules) students.
- `group_payment`: aggregated payment status for the group (class schedules only).

### Preview Request Example

```json
{
  "schedule_name": "IELTS Intensive",
  "schedule_type": "class",
  "group_id": 51,
  "recurring_pattern": "custom",
  "total_hours": 36,
  "hours_per_session": 2,
  "session_per_week": 3,
  "start_date": "2025-10-07T00:00:00Z",
  "estimated_end_date": "2025-11-18T00:00:00Z",
  "session_times": [
    { "weekday": 1, "start_time": "09:00" },
    { "weekday": 3, "start_time": "09:00" },
    { "weekday": 5, "start_time": "09:00" }
  ],
  "default_teacher_id": 23,
  "default_room_id": 4,
  "auto_reschedule": true
}
```

### Preview Response (trimmed)

```json
{
  "can_create": false,
  "issues": [
    {
      "severity": "error",
      "code": "teacher_conflict",
      "message": "teacher JaneDoe has 1 conflicting session(s)",
      "details": {
        "teacher_id": 23,
        "teacher_name": "JaneDoe",
        "conflicts": [
          {
            "schedule_id": 440,
            "schedule_name": "IELTS Intensive (Existing)",
            "session_id": 9123,
            "session_date": "2025-10-14",
            "start_time": "09:00",
            "end_time": "11:00"
          }
        ]
      }
    },
    {
      "severity": "warning",
      "code": "holiday_overlap",
      "message": "some sessions fall on Thai public holidays",
      "details": [
        {
          "session_number": 6,
          "date": "2025-12-05",
          "shifted_to": "2025-12-06",
          "was_rescheduled": true
        }
      ]
    }
  ],
  "summary": {
    "schedule_name": "IELTS Intensive",
    "schedule_type": "class",
    "start_date": "2025-10-07",
    "estimated_end_date": "2025-11-18",
    "total_hours": 36,
    "hours_per_session": 2,
    "session_per_week": 3,
    "total_sessions": 18
  },
  "sessions": [
    {
      "session_number": 1,
      "week_number": 1,
      "date": "2025-10-07",
      "start_time": "09:00",
      "end_time": "11:00"
    },
    "..."
  ],
  "holiday_impacts": [
    {
      "session_number": 6,
      "date": "2025-12-05",
      "shifted_to": "2025-12-06",
      "was_rescheduled": true
    }
  ],
  "conflicts": {
    "group": null,
    "rooms": [
      {
        "room_id": 4,
        "conflicts": [
          {
            "room_id": 4,
            "existing_room_id": 4,
            "session_id": 9123,
            "schedule_id": 440,
            "schedule_name": "IELTS Intensive (Existing)",
            "session_date": "2025-10-14",
            "start_time": "09:00",
            "end_time": "11:00"
          }
        ]
      }
    ],
    "teachers": [
      {
        "teacher_id": 23,
        "teacher_name": "JaneDoe",
        "conflicts": [
          {
            "schedule_id": 440,
            "schedule_name": "IELTS Intensive (Existing)",
            "session_id": 9123,
            "session_date": "2025-10-14",
            "start_time": "09:00",
            "end_time": "11:00"
          }
        ]
      }
    ],
    "participants": [],
    "students": []
  },
  "group_payment": {
    "group_id": 51,
    "group_name": "IELTS Intensive",
    "group_payment_status": "deposit_paid",
    "eligible_members": 3,
    "ineligible_members": 1,
    "member_totals": {
      "pending": 1,
      "deposit_paid": 2,
      "fully_paid": 1
    },
    "require_deposit": true
  },
  "auto_reschedule": true,
  "branch_hours": {
    "open_minutes": 480,
    "close_minutes": 1260,
    "open_time": "08:00",
    "close_time": "21:00"
  },
  "checked_room_ids": [4]
}
```

Use the preview output to highlight blockers in the UI before sending the final create request.

### Non-class preview example

The preview and room-conflict endpoints accept the same structure when you're creating `meeting`, `event`, `appointment`, or `holiday` schedules. Typical differences versus class creation:

- `group_id` is optional or omitted entirely.
- Participants are supplied by `participant_user_ids`; the preview will warn if you skip them, but `POST /api/schedules` still requires them at creation time.
- Student payment summaries are excluded.

Preview request for a meeting schedule:

```json
{
  "schedule_name": "Admin Sync",
  "schedule_type": "meeting",
  "recurring_pattern": "weekly",
  "total_hours": 4,
  "hours_per_session": 2,
  "session_per_week": 1,
  "start_date": "2025-10-10T00:00:00Z",
  "estimated_end_date": "2025-11-07T00:00:00Z",
  "session_start_time": "14:00",
  "default_teacher_id": 5,
  "default_room_id": 12,
  "participant_user_ids": [1, 7, 9]
}
```

Room conflict check for the same plan:

```json
{
  "room_id": 12,
  "recurring_pattern": "weekly",
  "total_hours": 4,
  "hours_per_session": 2,
  "session_per_week": 1,
  "start_date": "2025-10-10T00:00:00Z",
  "estimated_end_date": "2025-11-07T00:00:00Z",
  "session_start_time": "14:00"
}
```

Responses reuse the same shape highlighted above (`issues`, `summary`, `conflicts.rooms`, `conflicts.participants`, etc.), so UI integrations can display a unified preview regardless of schedule type.

## Troubleshooting Checklist

- Branch hours missing? Seed `open_time` / `close_time` for each branch in the database migration.
- Unexpected session count? Recalculate `total_hours ÷ hours_per_session` and ensure `session_per_week` matches your slot count.
- Holiday pushback not desired? Disable by setting `auto_reschedule` to `false`.
- Duplicate weekday rejection? Ensure `session_times` uses unique `weekday` values per week.
