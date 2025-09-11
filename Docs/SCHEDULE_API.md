# Schedule Management API Documentation

## Overview
Schedule Management API สำหรับจัดการตารางเรียนและ sessions ในระบบ English Korat

## Authentication
ทุก API endpoints ต้องการ JWT token ใน Authorization header:
```
Authorization: Bearer <JWT_TOKEN>
```

## Schedule Endpoints

### 1. Create Schedule
**POST** `/api/schedules`

**Permissions:** Admin, Owner เท่านั้น

**Request Body:**
```json
{
  "course_id": 1,
  "assigned_to_teacher_id": 5,
  "room_id": 2,
  "schedule_name": "English Class A1",
  "schedule_type": "class",
  "recurring_pattern": "weekly",
  "total_hours": 40,
  "hours_per_session": 2,
  "session_per_week": 2,
  "max_students": 10,
  "start_date": "2025-01-01T00:00:00Z",
  "estimated_end_date": "2025-05-01T00:00:00Z",
  "auto_reschedule": true,
  "notes": "Basic English course for beginners",
  "user_in_course_ids": [3, 4, 6, 7],
  "session_start_time": "09:00",
  "custom_recurring_days": [1, 3, 5]
}
```

**Response:**
```json
{
  "message": "Schedule created successfully",
  "schedule": {
    "id": 1,
    "course_id": 1,
    "assigned_to_teacher_id": 5,
    "room_id": 2,
    "schedule_name": "English Class A1",
    "schedule_type": "class",
    "status": "assigned",
    "created_at": "2025-01-01T00:00:00Z",
    "course": {...},
    "assigned_to": {...},
    "room": {...}
  }
}
```

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

## Schedule Types
- `class`: คลาสเรียนปกติ
- `meeting`: การประชุม
- `event`: กิจกรรม
- `holiday`: วันหยุด
- `appointment`: การนัดหมาย

## Recurring Patterns
- `daily`: ทุกวัน
- `weekly`: ทุกสัปดาห์
- `bi-weekly`: ทุก 2 สัปดาห์
- `monthly`: ทุกเดือน
- `yearly`: ทุกปี
- `custom`: กำหนดเอง (ใช้ `custom_recurring_days`)

## Session Status
- `scheduled`: กำหนดการแล้ว
- `confirmed`: ยืนยันแล้ว (ครูยืนยัน)
- `pending`: รอการยืนยัน
- `completed`: เสร็จสิ้นแล้ว
- `cancelled`: ยกเลิก
- `rescheduled`: เลื่อนการ
- `no-show`: ไม่มาเรียน

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
- แจ้งเตือนครูเมื่อได้รับมอบหมาย schedule ใหม่
- แจ้งเตือนนักเรียนเมื่อครูยืนยัน schedule
- แจ้งเตือนก่อนเรียน 30 นาที และ 1 ชั่วโมง (เฉพาะ session ที่ confirmed)
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
```bash
curl -X POST http://localhost:8080/api/schedules \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "course_id": 1,
    "assigned_to_teacher_id": 5,
    "room_id": 2,
    "schedule_name": "English Class A1",
    "schedule_type": "class",
    "recurring_pattern": "weekly",
    "total_hours": 40,
    "hours_per_session": 2,
    "session_per_week": 2,
    "max_students": 10,
    "start_date": "2025-01-01T00:00:00Z",
    "estimated_end_date": "2025-05-01T00:00:00Z",
    "auto_reschedule": true,
    "user_in_course_ids": [3, 4, 6, 7],
    "session_start_time": "09:00"
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
