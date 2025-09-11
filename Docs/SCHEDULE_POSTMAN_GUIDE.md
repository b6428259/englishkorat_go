# English Korat - Schedule API Postman Collection

คู่มือการใช้งาน Postman Collection สำหรับ Schedule API ของระบบ English Korat

## 📁 ไฟล์ที่ต้องใช้

1. **Collection:** `English_Korat_Schedule_API.postman_collection.json`
2. **Environment:** `English_Korat_Schedule_API.postman_environment.json`

## 🚀 การติดตั้งและตั้งค่า

### 1. Import Collection และ Environment

```bash
# วิธีที่ 1: Import ผ่าน Postman UI
1. เปิด Postman
2. คลิก "Import" 
3. เลือกไฟล์ Collection และ Environment
4. คลิก "Import"

# วิธีที่ 2: Import ผ่าน URL (ถ้าเก็บใน Git)
1. คลิก "Import" > "Link"
2. ใส่ URL ของไฟล์
```

### 2. ตั้งค่า Environment

```json
{
  "base_url": "http://localhost:8080",
  "auth_token": "",
  "schedule_id": "",
  "session_id": "",
  "course_id": "1",
  "teacher_id": "2", 
  "room_id": "1",
  "branch_id": "1"
}
```

### 3. เลือก Environment

1. ที่มุมขวาบนของ Postman
2. เลือก "English Korat Schedule - Development"

## 🔐 การ Authentication

### ขั้นตอนการเข้าสู่ระบบ:

1. **เริ่มต้น:** ไปที่ folder "Authentication"
2. **รัน:** "Login (Get Token)" request
3. **แก้ไข credentials ในตัวอย่าง:**
   ```json
   {
     "username": "admin",
     "password": "password123"
   }
   ```
4. **Auto-save:** Token จะถูกบันทึกใน Environment อัตโนมัติ

### User Roles และสิทธิ์:

- **Owner/Admin:** สิทธิ์เต็ม (สร้าง, ดู, แก้ไข, ลบทุกอย่าง)
- **Teacher:** ยืนยันตารางที่ถูกมอบหมาย, อัพเดท session, สร้าง makeup
- **Student:** ดูตารางที่เข้าร่วมเท่านั้น

## 📚 การใช้งาน API

### 1. Schedule Management

#### 🏗️ สร้างตารางเรียน (Create Schedule)
```http
POST /api/schedules
Authorization: Bearer {{auth_token}}
```

**ตัวอย่าง Request Body:**
```json
{
  "course_id": 1,
  "assigned_to_teacher_id": 2,
  "room_id": 1,
  "schedule_name": "English Grammar Class A",
  "schedule_type": "class",
  "recurring_pattern": "weekly",
  "total_hours": 20,
  "hours_per_session": 2,
  "session_per_week": 2,
  "max_students": 15,
  "start_date": "2025-09-15T00:00:00Z",
  "estimated_end_date": "2025-11-15T00:00:00Z",
  "auto_reschedule": true,
  "notes": "Beginning grammar class",
  "user_in_course_ids": [3, 4, 5],
  "session_start_time": "09:00",
  "custom_recurring_days": [1, 3]
}
```

**สำคัญ:** 
- `custom_recurring_days`: [0=อาทิตย์, 1=จันทร์, 2=อังคาร, ..., 6=เสาร์]
- `schedule_type`: class, meeting, event, holiday, appointment
- `recurring_pattern`: daily, weekly, bi-weekly, monthly, yearly, custom

#### ✅ ยืนยันตารางเรียน (Confirm Schedule)
```http
PATCH /api/schedules/{{schedule_id}}/confirm
```

**เฉพาะครูที่ถูก assign เท่านั้น**

#### 📋 ดูตารางเรียน

1. **ดูตารางตัวเอง:**
   ```http
   GET /api/schedules/my
   ```

2. **ดูทั้งหมด (Admin):**
   ```http
   GET /api/schedules?status=assigned&type=class&branch_id=1
   ```

### 2. Session Management

#### 📅 ดู Sessions ของตารางเรียน
```http
GET /api/schedules/{{schedule_id}}/sessions
```

#### 🔄 อัพเดทสถานะ Session
```http
PATCH /api/schedules/sessions/{{session_id}}/status
```

**Status Options:**
- `confirmed`: ยืนยันการเรียน
- `completed`: เรียนเสร็จแล้ว  
- `cancelled`: ยกเลิก
- `no-show`: นักเรียนไม่มา

#### 🔧 สร้าง Makeup Session
```http
POST /api/schedules/sessions/makeup
```

**ใช้ได้เฉพาะ schedule_type = "class"**

### 3. Comment System

#### 💬 เพิ่ม Comment
```http
POST /api/schedules/comments
```

**สำหรับ Schedule:**
```json
{
  "schedule_id": 1,
  "comment": "This schedule looks good for beginners"
}
```

**สำหรับ Session:**
```json
{
  "session_id": 1, 
  "comment": "Great participation today"
}
```

#### 👁️ ดู Comments
```http
# ดู comments ของ schedule
GET /api/schedules/comments?schedule_id=1

# ดู comments ของ session  
GET /api/schedules/comments?session_id=1
```

## 🎯 ตัวอย่างการใช้งาน

### Scenario 1: สร้างคอร์สเรียนประจำ

1. **Login** ด้วย admin account
2. **สร้างตารางเรียน** ด้วย "Weekly Schedule Example"
3. **Teacher login** และ **confirm schedule**
4. **ดู sessions** ที่ถูกสร้างอัตโนมัติ
5. **เริ่มเรียน:** อัพเดท session เป็น "confirmed"

### Scenario 2: จัดการ Makeup Session

1. **อัพเดท session** เป็น "cancelled" พร้อมเหตุผล
2. **สร้าง makeup session** ด้วยวันเวลาใหม่
3. **เพิ่ม comment** บันทึกการเปลี่ยนแปลง

### Scenario 3: ติดตามความคืบหน้า

1. **ดูตารางเรียนของตัวเอง** (`/my`)
2. **ดู sessions** ของแต่ละตารางเรียน
3. **อ่าน comments** ของแต่ละ session
4. **เพิ่ม comment** บันทึกผลการเรียน

## 🔧 Tips การใช้งาน

### 1. Auto-save IDs
Collection ถูกตั้งค่าให้บันทึก ID อัตโนมัติ:
- `schedule_id` จาก response การสร้าง schedule
- `session_id` จาก response ดู sessions
- `auth_token` จาก login response

### 2. Error Handling
ทุก request มี test script ที่:
- ตรวจสอบ status code
- Validate JSON response
- Log ข้อมูลเพื่อ debug

### 3. Environment Variables
ใช้ตัวแปรเหล่านี้ในการทดสอบ:
```
{{base_url}}         # http://localhost:8080
{{auth_token}}       # JWT token
{{schedule_id}}      # ID ของตารางเรียนล่าสุด
{{session_id}}       # ID ของ session ล่าสุด
{{course_id}}        # ID ของคอร์สสำหรับทดสอบ
{{teacher_id}}       # ID ของครูสำหรับทดสอบ
{{room_id}}          # ID ของห้องสำหรับทดสอบ
```

## 🐛 Troubleshooting

### ปัญหาที่อาจพบ:

1. **401 Unauthorized**
   - ตรวจสอบ auth_token ใน environment
   - ทำการ login ใหม่

2. **403 Forbidden** 
   - ตรวจสอบสิทธิ์ของ user role
   - Admin/Owner เท่านั้นที่สร้าง schedule ได้

3. **400 Bad Request - Room Conflict**
   - เปลี่ยน room_id หรือเวลา
   - ตรวจสอบห้องว่าง

4. **404 Not Found**
   - ตรวจสอบ ID ใน environment variables
   - ตรวจสอบว่า resource มีอยู่จริง

### การ Debug:

1. **เปิด Postman Console** (View > Show Postman Console)
2. **ดู Request/Response** ใน console
3. **ตรวจสอบ Environment Variables**
4. **ลองใช้ Collection แบบ step-by-step**

## 📞 Support

หากพบปัญหาหรือต้องการความช่วยเหลือ:
- ตรวจสอบ API Documentation: `SCHEDULE_API.md`
- ตรวจสอบ logs ใน console
- ทดสอบด้วย manual request ก่อน

---

## 🎉 Ready to Use!

Collection นี้ครอบคลุมการใช้งาน Schedule API ทั้งหมด พร้อมตัวอย่างและการจัดการ error ที่สมบูรณ์
