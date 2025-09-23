# Schedule Management API Documentation

## Overview
Schedule Management API สำหรับจัดการตารางเรียนและ sessions ในระบบ English Korat

## Authentication
ทุก API endpoints ต้องการ JWT token ใน Authorization header:
```
Authorization: Bearer <JWT_TOKEN>
```

## Status semantics (quick guide)
- Session status:
  - assigned = สำหรับคลาส (class) หมายถึงรอครูยืนยันคาบเรียน
  - scheduled = ยืนยันแล้ว (สถานะหลังครูยืนยันผ่าน API ยืนยันคาบเรียน)
  - อื่น ๆ (confirmed/pending/completed/...) มีไว้สำหรับการใช้งานเฉพาะกรณีด้วย API อื่น แต่ระบบอัตโนมัติจะถือว่า "scheduled" คือยืนยันแล้ว
- Schedule status:
  - assigned = มอบหมายแล้ว (อาจรอการยืนยันระดับตาราง)
  - scheduled = ตารางได้รับการยืนยันแล้ว

## Schedule Endpoints

### 1. Create Schedule
**POST** `/api/schedules`

**Permissions:** Admin, Owner เท่านั้น

**Request Body (Class):**
```json
{
  "schedule_name": "English Class A1",
  "schedule_type": "class",
  "group_id": 10,
  "recurring_pattern": "weekly",
  "total_hours": 40,
  "hours_per_session": 2,
  "session_per_week": 2,
  "start_date": "2025-01-01T00:00:00Z",
  "estimated_end_date": "2025-05-01T00:00:00Z",
  "default_teacher_id": 7,
  "default_room_id": 3,
  "auto_reschedule": true,
  "notes": "Basic English course for beginners",
  "session_start_time": "09:00",
  "custom_recurring_days": [1, 3, 5]
}
```

Notes:
- For class schedules, `group_id` is required and participants are implicitly the group members.
- Default teacher/room can be set; each session may override later.
- Notification: เมื่อสร้าง schedule แบบ class ระบบจะส่ง normal notification ไปยังครูที่ถูกมอบหมายให้ตรวจสอบคาบเรียน (link ชี้ไปที่ `/api/schedules/{id}/sessions`)

**Response (Class):**
```json
{
  "message": "Schedule created successfully",
  "schedule": {
    "id": 1,
    "schedule_name": "English Class A1",
    "schedule_type": "class",
    "status": "assigned",
    "created_at": "2025-01-01T00:00:00Z",
    "group": {"id": 10, "course": {"name": "Basic English", "level": "A1"}},
    "default_teacher": {"id": 7, "username": "teacherA"},
    "default_room": {"id": 3, "name": "Room 101"}
  }
}
```

**Request Body (Meeting/Event/Appointment):**
```json
{
  "schedule_name": "Team Meeting",
  "schedule_type": "meeting",
  "participant_user_ids": [12, 13],
  "recurring_pattern": "weekly",
  "total_hours": 4,
  "hours_per_session": 2,
  "session_per_week": 1,
  "start_date": "2025-09-01T00:00:00Z",
  "estimated_end_date": "2025-10-01T00:00:00Z",
  "default_teacher_id": 7,
  "default_room_id": 3,
  "session_start_time": "09:00",
  "custom_recurring_days": [2]
}
```
Notes:
- For non-class schedules, specify `participant_user_ids`. Organizer is set to the creator.
- Notification: สำหรับ non-class schedules ระบบจะส่งคำเชิญ (popup + normal) ไปยังผู้เข้าร่วมทุกคน พร้อม deep-link ไปที่ `/api/schedules/{id}`

### 2. Confirm Schedule
**PATCH** `/api/schedules/:id/confirm`

**Permissions:** เฉพาะครูที่ถูก assign

**Request Body:**
```json
{
  "status": "scheduled"
}
```

**Response:**
```json
{
  "message": "Schedule confirmed successfully",
  "schedule": {...}
}
```

### 3. Get My Schedules
**GET** `/api/schedules/my`

**Permissions:** Teacher, Student (เห็นข้อมูลต่างกัน)

**Response:**
```json
{
  "schedules": [
    {
      "id": 1,
      "schedule_name": "English Class A1",
      "schedule_type": "class",
      "status": "scheduled",
      "start_date": "2025-01-01T00:00:00Z",
      "course": {
        "name": "Basic English",
        "level": "A1"
      },
      "room": {...}
    }
  ]
}
```

### 4. Get All Schedules (Admin/Owner)
**GET** `/api/schedules`

**Permissions:** Admin, Owner เท่านั้น

**Query Parameters:**
- `status`: Filter by status (assigned, scheduled, completed, etc.)
- `type`: Filter by schedule type (class, meeting, event, etc.)
- `branch_id`: Filter by branch

**Response:**
```json
{
  "schedules": [...]
}
```

### 5. Get Schedule Sessions
**GET** `/api/schedules/:id/sessions`

**Response:**
```json
{
  "sessions": [
    {
      "id": 1,
      "schedule_id": 1,
      "session_date": "2025-01-01",
      "start_time": "2025-01-01T09:00:00Z",
      "end_time": "2025-01-01T11:00:00Z",
      "session_number": 1,
      "week_number": 1,
      "status": "scheduled",
      "is_makeup": false,
      "notes": ""
    }
  ]
}
```

### 6. Update Session Status
**PATCH** `/api/schedules/sessions/:id/status`

**Permissions:** Teacher ที่ถูก assign, Admin, Owner

**Request Body:**
```json
{
  "status": "confirmed",
  "notes": "Session confirmed by teacher"
}
```

**Response:**
```json
{
  "message": "Session status updated successfully",
  "session": {...}
}
```

### 7. Create Makeup Session
**POST** `/api/schedules/sessions/makeup`

**Permissions:** Teacher ที่ถูก assign, Admin, Owner

**Request Body:**
```json
{
  "original_session_id": 5,
  "new_session_date": "2025-01-10T00:00:00Z",
  "new_start_time": "14:00",
  "cancelling_reason": "Teacher was sick",
  "new_session_status": "cancelled"
}
```

**Response:**
```json
{
  "message": "Makeup session created successfully",
  "makeup_session": {...}
}
```

## Comment Endpoints

### 8. Add Comment
**POST** `/api/schedules/comments`

**Request Body:**
```json
{
  "schedule_id": 1,
  "comment": "Student shows good progress"
}
```
หรือ
```json
{
  "session_id": 5,
  "comment": "Session went well, all students participated"
}
```

**Response:**
```json
{
  "message": "Comment added successfully",
  "comment": {
    "id": 1,
    "schedule_id": 1,
    "user_id": 5,
    "comment": "Student shows good progress",
    "created_at": "2025-01-01T00:00:00Z",
    "user": {...}
  }
}
```

### 9. Get Comments
**GET** `/api/schedules/comments`

**Query Parameters:**
- `schedule_id`: Get comments for specific schedule
- `session_id`: Get comments for specific session

**Response:**
```json
{
  "comments": [
    {
      "id": 1,
      "schedule_id": 1,
      "user_id": 5,
      "comment": "Student shows good progress",
      "created_at": "2025-01-01T00:00:00Z",
      "user": {
        "username": "teacher1",
        "role": "teacher"
      }
    }
  ]
}
```

### 10. Get Schedule Detail (Normalized)
**GET** `/api/schedules/:id`

Notes:
- Response is normalized to avoid deep nested GORM structs and reduce duplication.
- All relevant relations are preloaded: group/course/members (for class), created_by, default_teacher, default_room, sessions (with assigned teacher/room), and participants for non-class schedules.

Response:
```json
{
  "success": true,
  "data": {
    "id": 48,
    "created_at": "2025-09-23T10:00:00Z",
    "updated_at": "2025-09-23T10:00:00Z",
    "schedule_name": "Team Meeting",
    "schedule_type": "meeting",
    "status": "assigned",
    "recurring_pattern": "weekly",
    "total_hours": 4,
    "hours_per_session": 2,
    "session_per_week": 1,
    "start_date": "2025-09-01T00:00:00Z",
    "estimated_end_date": "2025-10-01T00:00:00Z",
    "notes": "",
    "auto_reschedule": true,
    "created_by": { "id": 1, "username": "admin" },
    "default_teacher": { "id": 7, "username": "teacherA" },
    "default_room": { "id": 3, "name": "Room 101" },
    "group": null,
    "participants": [
      { "user_id": 12, "role": "organizer", "status": "invited", "user": { "id": 12, "username": "john" }},
      { "user_id": 13, "role": "participant", "status": "invited", "user": { "id": 13, "username": "jane" }}
    ],
    "sessions": [
      {
        "id": 2051,
        "schedule_id": 48,
        "date": "2025-09-24",
        "start_time": "09:00",
        "end_time": "11:00",
        "status": "scheduled",
        "session_number": 1,
        "week_number": 3,
        "is_makeup": false,
        "notes": "",
        "teacher": { "id": 7, "username": "teacherA" },
        "room": { "id": 3, "name": "Room 101" }
      }
    ]
  }
}
```

#### Class example response
```json
{
  "success": true,
  "data": {
    "id": 21,
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-01T00:00:00Z",
    "schedule_name": "English Class A1",
    "schedule_type": "class",
    "status": "assigned",
    "recurring_pattern": "weekly",
    "total_hours": 40,
    "hours_per_session": 2,
    "session_per_week": 2,
    "start_date": "2025-01-01T00:00:00Z",
    "estimated_end_date": "2025-05-01T00:00:00Z",
    "notes": "Basic English course for beginners",
    "auto_reschedule": true,
    "created_by": { "id": 1, "username": "admin" },
    "default_teacher": {"id": 7, "username": "teacherA"},
    "default_room": {"id": 3, "name": "Room 101"},
    "group": {
      "id": 10,
      "group_name": "A1-Group-01",
      "course": {"id": 5, "name": "Basic English", "level": "A1"},
      "members": [
        {"id": 1001, "student": {"id": 501, "first_name_en": "Somchai", "last_name_en": "Dee"}},
        {"id": 1002, "student": {"id": 502, "first_name_en": "Suda", "last_name_en": "Ying"}}
      ]
    },
    "participants": [],
    "sessions": [
      {"id": 301, "schedule_id": 21, "date": "2025-01-01", "start_time": "09:00", "end_time": "11:00", "status": "assigned", "session_number": 1, "week_number": 1, "teacher": {"id": 7, "username": "teacherA"}, "room": {"id": 3, "name": "Room 101"}},
      {"id": 302, "schedule_id": 21, "date": "2025-01-03", "start_time": "09:00", "end_time": "11:00", "status": "assigned", "session_number": 2, "week_number": 1, "teacher": {"id": 7, "username": "teacherA"}, "room": {"id": 3, "name": "Room 101"}}
    ]
  }
}
```

Example usage:
- Frontend can call http://localhost:3000/api/schedules/48 to show details. For actionable notifications with data.link.href, compose base_url + href and reuse this endpoint.

### 11. Add Session to Schedule
**POST** `/api/schedules/:id/sessions`

Add a new session into an existing schedule.

Permissions:
- Admin, Owner, or default teacher of the schedule.

Request Body:
```json
{
  "date": "2025-10-01",           // YYYY-MM-DD
  "start_time": "09:00",          // HH:MM
  "end_time": "11:00",            // optional; if omitted, uses schedule.hours_per_session
  "hours": 2,                       // optional override for duration hours when end_time omitted
  "assigned_teacher_id": 7,         // optional; defaults to schedule.default_teacher_id
  "room_id": 3,                     // optional; defaults to schedule.default_room_id when missing at session level
  "notes": "Extra session"
}
```

Behavior:
- Class schedule: created session starts with status "assigned" and requires teacher confirmation. Teacher receives popup+normal invitation to confirm; follow-up reminders are sent T-24h and T-6h before start if still unconfirmed.
- Non-class schedule: created session is immediately "scheduled".
- Notification payload for confirmation includes data.link.href to `/api/schedules/sessions/{id}` and action `confirm-session`.
 - Non-class: เมื่อเพิ่ม session จะมีการส่งแจ้งเตือน (popup + normal) ไปยังผู้เข้าร่วมทั้งหมด และครูที่ถูกกำหนดใน session (ถ้ามี)

Response:
```json
{
  "message": "Session added successfully",
  "session": { "id": 999, "schedule_id": 21, "status": "assigned", "session_number": 5, "week_number": 3, "date": "2025-10-01", "start_time": "09:00", "end_time": "11:00" }
}
```

### 12. Update My Participation Status (non-class)
**PATCH** `/api/schedules/:id/participants/me`

Update the current user’s participation status for a non-class schedule.

Permissions:
- Any participant of the target schedule (non-class only). Returns 400 for class schedules.

Request Body:
```json
{ "status": "confirmed" } // one of: confirmed | declined | tentative
```

Response:
```json
{
  "message": "Participation status updated",
  "participant": { "user_id": 12, "status": "confirmed" },
  "schedule_id": 740,
  "new_status": "confirmed"
}
```

### 13. Confirm Session (per-session confirmation)
**PATCH** `/api/schedules/sessions/:id/confirm`

Confirm a single session. For class schedules, this is the flowครูยืนยันคาบเรียนรายคาบ; สำหรับ non-class ผู้เข้าร่วมสามารถยืนยันได้เช่นกัน

Permissions:
- Admin, Owner
- Class: ครูที่ถูก assign ในคาบนั้น หรือ default teacher ของ schedule
- Non-class: ผู้เข้าร่วมของ schedule

Behavior:
- อัพเดทสถานะ session เป็น `scheduled` และบันทึก `confirmed_at` พร้อม `confirmed_by_user_id`

Response:
```json
{ "message": "Session confirmed successfully" }
```

## Schedule Types
- `class`: คลาสเรียนปกติ
- `meeting`: การประชุม
- `event`: กิจกรรม
- `holiday`: วันหยุด
- `appointment`: การนัดหมาย

## Recurring Patterns and Generation Rules

Supported patterns:
- `daily`: ทุกวัน
- `weekly`: ทุกสัปดาห์ (ตรงกับ weekday ของ `start_date`)
- `bi-weekly`: ทุก 2 สัปดาห์ (ตรงกับ weekday ของ `start_date` และเว้นสัปดาห์)
- `monthly`: ทุกเดือน (ตรงกับ day-of-month ของ `start_date`)
- `yearly`: สามารถกำหนดเป็นประเภท schedule ได้ แต่ตัว generator ในเวอร์ชันนี้รองรับหลัก ๆ ที่ระบุด้านบน
- `custom`: กำหนดเองด้วย `custom_recurring_days` (0=Sun, 1=Mon, ..., 6=Sat; ค่าที่เป็น 7 จะถูกแปลงเป็น 0)
- `none`: one-off (สร้าง session แรกเท่านั้น)

Computation details:
- จำนวน sessions = `total_hours / hours_per_session` (ปัดเศษลง)
- `week_number` จะเพิ่มทีละ 1 หลังจากครบ `session_per_week` ในแต่ละสัปดาห์
- Timezone: ใช้ Asia/Bangkok ในการ normalize วันที่และเวลา
- หากไม่ได้กำหนด `estimated_end_date` ระบบจะจำกัดการสร้างล่วงหน้าไว้ประมาณ 1 ปีเพื่อป้องกัน loop ไม่สิ้นสุด
- Holiday handling: เมื่อ `auto_reschedule` เป็น true จะดึงวันหยุดราชการไทย และเลื่อน session ที่ตรงวันหยุดไปวันถัดไป โดยคงเวลาเดิมไว้

Examples:
- weekly เริ่มวันพุธ (start_date = 2025-01-01) และ session_per_week = 2, custom_recurring_days = [1,3] (Mon, Wed) → ระบบจะสร้างคาบเรียนทุกจันทร์และพุธ โดย `session_number` นับต่อเนื่อง และ `week_number` เพิ่มหลังครบ 2 sessions ต่อสัปดาห์

## Session Status
- `scheduled`: กำหนดการแล้ว
- `confirmed`: ยืนยันแล้ว (ครูยืนยัน)
- `pending`: รอการยืนยัน
- `completed`: เสร็จสิ้นแล้ว
- `cancelled`: ยกเลิก
- `rescheduled`: เลื่อนการ
- `no-show`: ไม่มาเรียน

หมายเหตุ: ใน flow ปัจจุบัน การยืนยันคาบเรียนผ่าน endpoint ยืนยันคาบเรียน จะทำให้สถานะเป็น `scheduled` (ถือเป็นยืนยันแล้ว) ส่วนสถานะ `confirmed` รองรับผ่าน API อัปเดตสถานะทั่วไปและอาจใช้ในกรณีเฉพาะเท่านั้น

## Schedule Status
- `assigned`: มอบหมายแล้ว (รอครูยืนยัน)
- `scheduled`: กำหนดการแล้ว (ครูยืนยันแล้ว)
- `paused`: หยุดชั่วคราว
- `completed`: เสร็จสิ้นแล้ว
- `cancelled`: ยกเลิก

## Automatic Features

### 1. Room Conflict Detection
- ระบบจะตรวจสอบการชนของห้องเรียนอัตโนมัติสำหรับ schedule type "class"
- ป้องกันการจองห้องเดียวกันในเวลาที่ซ้อนทับ

### 2. Holiday Reschedule
- เมื่อเปิด `auto_reschedule: true` ระบบจะดึงวันหยุดไทยจาก API
- Sessions ที่ตรงกับวันหยุดจะถูกเลื่อนไปวันถัดไปโดยอัตโนมัติ

### 3. Automatic Notifications
- แจ้งเตือนครูเมื่อได้รับมอบหมาย schedule ใหม่ (class)
- สำหรับ class: ทุก session จะเริ่มต้นเป็นสถานะ "assigned" และต้องให้ครูยืนยันทีละ session
  - ระบบส่ง popup+normal เชิญให้ครูยืนยันทันทีหลังสร้าง
  - ระบบตั้งเตือนย้ำให้ครูยืนยันที่ T-24h และ T-6h ก่อนเริ่ม หากยังไม่ยืนยัน
  - เมื่อครูยืนยันแล้ว สถานะ session จะกลายเป็น "scheduled"
- แจ้งเตือนผู้มีส่วนเกี่ยวข้องเมื่อมีการสร้างหรือยืนยัน
  - Non-class: ผู้เข้าร่วมทั้งหมดได้รับคำเชิญ (popup + normal) เมื่อสร้าง schedule ใหม่ และเมื่อเพิ่ม session ใหม่
  - Class: แจ้งเตือนครู (normal) เมื่อได้รับมอบหมาย schedule ใหม่; นักเรียนในกลุ่มได้รับการแจ้งเตือนเมื่อ schedule ถูกยืนยัน (ระดับ schedule)
- แจ้งเตือนก่อนเรียน 30 นาที และ 1 ชั่วโมง สำหรับ session ที่สถานะ "scheduled"
- ส่งสรุปตารางเรียนประจำวันทุกเช้า เวลา 07:00
- แจ้งเตือน admin เมื่อมี session no-show

### 4. Session Generation
- ระบบจะสร้าง sessions อัตโนมัติตาม recurring pattern
- คำนวณจำนวน sessions จาก total_hours / hours_per_session
- จัดการ week_number และ session_number อัตโนมัติ

## Error Codes
- `400`: Bad Request - ข้อมูลไม่ถูกต้อง
- `401`: Unauthorized - ไม่มี token หรือ token หมดอายุ
- `403`: Forbidden - ไม่มีสิทธิ์เข้าถึง
- `404`: Not Found - ไม่พบข้อมูล
- `409`: Conflict - การชนของห้องเรียน
- `500`: Internal Server Error - ข้อผิดพลาดของระบบ

## Usage Examples

### 1. สร้าง Schedule ใหม่

ตัวอย่าง Class
```bash
curl -X POST http://localhost:8080/api/schedules \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "schedule_name": "English Class A1",
    "schedule_type": "class",
    "group_id": 10,
    "recurring_pattern": "weekly",
    "total_hours": 40,
    "hours_per_session": 2,
    "session_per_week": 2,
    "start_date": "2025-01-01T00:00:00Z",
    "estimated_end_date": "2025-05-01T00:00:00Z",
    "default_teacher_id": 7,
    "default_room_id": 3,
    "auto_reschedule": true,
    "session_start_time": "09:00",
    "custom_recurring_days": [1,3]
  }'
```

ตัวอย่าง Meeting/Event
```bash
curl -X POST http://localhost:8080/api/schedules \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "schedule_name": "Team Meeting",
    "schedule_type": "meeting",
    "participant_user_ids": [12,13],
    "recurring_pattern": "weekly",
    "total_hours": 4,
    "hours_per_session": 2,
    "session_per_week": 1,
    "start_date": "2025-09-01T00:00:00Z",
    "estimated_end_date": "2025-10-01T00:00:00Z",
    "default_teacher_id": 7,
    "default_room_id": 3,
    "session_start_time": "09:00",
    "custom_recurring_days": [2]
  }'
```

### 2. ครูยืนยัน Schedule
```bash
curl -X PATCH http://localhost:8080/api/schedules/1/confirm \
  -H "Authorization: Bearer TEACHER_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "scheduled"}'
```

### 3. สร้าง Makeup Session
```bash
curl -X POST http://localhost:8080/api/schedules/sessions/makeup \
  -H "Authorization: Bearer TEACHER_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "original_session_id": 5,
    "new_session_date": "2025-01-10T00:00:00Z",
    "new_start_time": "14:00",
    "cancelling_reason": "Teacher was sick",
    "new_session_status": "cancelled"
  }'
```
