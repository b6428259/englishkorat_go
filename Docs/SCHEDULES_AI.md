# Schedules & Schedule Sessions — AI Reference

เอกสารฉบับนี้อธิบายโครงสร้างข้อมูลและพฤติกรรมของ `Schedules` และ `Schedule_Sessions` ในระบบ EnglishKorat (สำหรับให้ AI อ่านและเข้าใจการทำงานของโมดูลตารางสอน)

วัตถุประสงค์
- ให้ AI เข้าใจรูปแบบข้อมูล (schema), ความสัมพันธ์ (relationships), พฤติกรรม (business rules) และ edge-cases ที่สำคัญ
- ให้ตัวอย่าง JSON/payload สำหรับ API CRUD และ workflow ที่พบบ่อย
- ให้รายละเอียด validation rules และ default behaviors

แนะนำโดยย่อ
- `Schedules` แทนการวางแผนคอร์ส/ชั้นเรียน (เช่น คอร์ส TOEIC, Conversation) มีค่าเริ่มต้นและสถานะของคอร์ส
- `Schedule_Sessions` เป็น session แต่ละครั้งที่เกิดขึ้นภายใต้ `Schedules` (เช่น session วันที่ 2025-09-15 เวลา 18:00-19:30)

---

## 1) โมเดลและฟิลด์สำคัญ

A. Schedules (ตารางหลัก)
- id (uint) — primary key
- created_at, updated_at, deleted_at — timestamps
- course_id (uint) — FK -> Course
- user_in_course_id (uint) — FK -> User_inCourse (ผู้สร้างหรือ context ของการลงทะเบียน)
- assigned_to_user_id (uint) — FK -> users (ครูที่ถูกมอบหมาย)
- room_id (uint) — FK -> Room
- schedule_name (string) — ชื่อ schedule
- schedule_type (string) — type เช่น `class`, `meeting`, `event`, `holiday`, `appointment`
- recurring_pattern (string) — รูปแบบการทำซ้ำ เช่น `daily`, `weekly`, `bi-weekly`, `monthly`, `yearly`, `custom`
- total_hours (int)
- hours_per_session (int)
- session_per_week (int)
- max_students (int)
- current_students (int)
- start_date (Date)
- estimated_end_date (Date)
- actual_end_date (Date|nullable)
- status (string) — enum: `scheduled`, `paused`, `completed`, `cancelled`, `assigned` (default: `scheduled`)
- auto_reschedule (bool) — ถ้ามีวันหยุดให้ปรับตารางอัตโนมัติ
- notes (text)
- admin_assigned (string) — ชื่อ admin ที่ assign

B. Schedule_Sessions (session แต่ละครั้ง)
- schedule_id (uint) — FK -> Schedules
- session_date (date) — วันที่ session (ใช้ date-only หรือ datetime ขึ้นกับ implementation)
- start_time (time/datetime) — เวลาเริ่ม
- end_time (time/datetime) — เวลาจบ
- session_number (int) — ครั้งที่เท่าไรใน sequence
- week_number (int) — หมายเลขสัปดาห์ในคอนเท็กซ์ recurring
- status (string) — enum: `scheduled`, `confirmed`, `pending`, `completed`, `cancelled`, `rescheduled`, `no-show`
- cancelling_reason (text) — เหตุผลการยกเลิก (nullable)
- is_makeup (bool) — เป็นชดเชยหรือไม่
- makeup_for_session_id (*uint) — ถ้าเป็นชดเชย ชี้ไปยัง session ต้นทาง
- notes (text)

---

## 2) ความสัมพันธ์ (Relationships)
- Schedules 1 - N Schedule_Sessions
- Schedules -> Course (N:1)
- Schedules -> Room (N:1)
- Schedules -> User (ครู assigned) (N:1)
- Schedule_Sessions -> Schedule (N:1)
- Schedule_Sessions.makeup_for_session_id -> Schedule_Sessions.id (nullable self reference)

---

## 3) พฤติกรรมเชิงธุรกิจและ rules
1. การสร้าง Schedule
   - ต้องมี `schedule_name`, `start_date`, `schedule_type` อย่างน้อย
   - ถ้าเป็น recurring pattern เช่น `weekly` หรือ `daily` ต้องใช้ `session_per_week`, `hours_per_session` และ `total_hours` เพื่อคำนวณจำนวน session และ estimated_end_date
   - `assigned_to_user_id` เป็น optional; หากกำหนดจะถือว่า schedule ถูกมอบหมายให้ครูคนนั้น

2. การสร้าง Schedule_Sessions (auto-generated หรือ manual)
   - เมื่อสร้าง schedule แบบ recurring ระบบอาจสร้าง sessions ล่วงหน้าตาม pattern
   - `session_date` และ `start_time`/`end_time` ต้องไม่ขัดกับ `room` หรือ teacher availability (validation layer)
   - `session_number` ถูกใช้เพื่อแสดงตำแหน่งของ session ใน sequence และควรเพิ่มตามลำดับ

3. สถานะ (status) ของ sessions และ schedule
   - Session `status` เริ่มที่ `scheduled`
   - ครูหรือ admin สามารถ `confirm` session -> `confirmed`
   - หากผู้เรียน/ครูไม่มา -> `no-show`
   - ยกเลิก session -> `cancelled` และบันทึก `cancelling_reason`
   - เมื่อ session ย้าย -> `rescheduled` และเก็บ relation ของ session เก่า/ใหม่
   - Schedule จะเปลี่ยนเป็น `completed` เมื่อ sessions ทั้งหมดทำรายการ `completed` หรือเมื่อ `actual_end_date` ถูกตั้ง

4. Makeup sessions
   - หากเป็น `is_makeup = true` ให้ `makeup_for_session_id` ชี้ไป session เดิม
   - Makeup sessions ควรถูก link กับ schedule เดิมและมี `session_number` ใหม่

5. Auto-reschedule
   - หาก `auto_reschedule` true และระบบเจอวันหยุดหรือ conflict ระบบอาจเลื่อน session ไปยังช่วงเวลาว่างถัดไปตามกฎของ branch/teacher availability

---

## 4) API payload examples

A. Create Schedule (POST /api/schedules)
```json
{
  "course_id": 47,
  "schedule_name": "TOEIC Foundation - Batch A",
  "schedule_type": "class",
  "recurring_pattern": "weekly",
  "hours_per_session": 1,
  "session_per_week": 2,
  "total_hours": 30,
  "start_date": "2025-09-22",
  "assigned_to_user_id": 8,
  "room_id": 1,
  "notes": "Evening classes"
}
```

Expected behavior: server creates schedule record and can generate initial `Schedule_Sessions` for the next N weeks (N = total_hours / hours_per_session), or defer to worker.

B. Example generated Schedule_Session
```json
{
  "schedule_id": 101,
  "session_date": "2025-09-22",
  "start_time": "2025-09-22T18:00:00Z",
  "end_time": "2025-09-22T19:00:00Z",
  "session_number": 1,
  "week_number": 1,
  "status": "scheduled",
  "is_makeup": false
}
```

C. Update session (PATCH /api/schedules/sessions/:id)
- Confirm: set `status = confirmed`
- Cancel: set `status = cancelled` and supply `cancelling_reason`
- Reschedule: create a new session with `status = rescheduled` linking to old one

---

## 5) Validation rules & Edge cases for AI
- Dates: prefer ISO 8601 date for `start_date` and `session_date` (YYYY-MM-DD). `start_time`/`end_time` use full RFC3339 datetimes.
- Timezone: store in UTC or ensure timezone normalization when saving and comparing.
- Overlaps: do not allow overlapping sessions in the same `room_id` or for the same `assigned_to_user_id` (teacher) unless specifically allowed.
- Partial weeks: when `total_hours` isn't divisible by `hours_per_session`, last session may be shorter or schedule may extend an extra session.
- Makeup sessions should not increment `current_students` unless actual attendance occurs.
- Deleting a schedule should soft-delete sessions, not remove attendance history.
- Rescheduling should preserve attendance history by referencing the original session where appropriate.

---

## 6) Typical flows (summary) for AI
1. Admin creates schedule (recurring weekly) → system optionally generates sessions for upcoming weeks → notify assigned teacher and enrolled students.
2. Teacher confirms a session → `status=confirmed`.
3. Teacher cancels session → `status=cancelled`, set `cancelling_reason` → optionally create makeup session.
4. Student no-show → mark `no-show` in session (tracked separately in attendance), admin may schedule makeup.
5. When all sessions `completed` → mark schedule `completed` and set `actual_end_date`.

---

## 7) Prompts and queries AI can answer with this doc
- "Generate the next 10 sessions for this schedule starting from X"
- "Given teacher availability and room availability, suggest new time slots for rescheduling session #N"
- "Explain why a schedule would be marked `paused` and recommend next steps"

---

## 8) Notes for implementers
- Implement robust conflict detection for teacher/room availability before creating sessions.
- Keep `Schedule_Sessions` immutable for historical fields like attendance; use status and references for rescheduling.
- Use background workers for heavy operations (generating sessions, auto-rescheduling, notifications).


---

End of document — prepared for AI consumption (compact, rule-focused, and example-driven).
