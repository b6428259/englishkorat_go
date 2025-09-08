# English Korat API - Postman Collection

## 📁 ไฟล์ที่สร้างให้

1. **`English_Korat_API.postman_collection.json`** - Postman Collection หลัก
2. **`English_Korat_API.postman_environment.json`** - Environment Variables
3. **`POSTMAN_SETUP.md`** - คู่มือนี้

## 🚀 วิธีการ Import

### 1. Import Collection
1. เปิด Postman
2. คลิก **Import** 
3. เลือกไฟล์ `English_Korat_API.postman_collection.json`
4. คลิก **Import**

### 2. Import Environment
1. คลิก **Import** อีกครั้ง
2. เลือกไฟล์ `English_Korat_API.postman_environment.json`
3. คลิก **Import**
4. เลือก Environment "English Korat API Environment" ที่มุมขวาบน

## 🔐 การใช้งาน Authentication

### ขั้นตอนการ Login
1. ไปที่ Collection **🔐 Authentication → Login**
2. แก้ไข body ใน request:
   ```json
   {
     "username": "admin",
     "password": "password123"
   }
   ```
3. กด **Send**
4. **Token จะถูกเก็บอัตโนมัติ** ใน Environment Variables
5. Requests อื่น ๆ จะใช้ token นี้โดยอัตโนมัติ

### Script อัตโนมัติเมื่อ Login สำเร็จ
```javascript
// Script จะทำงานอัตโนมัติ
if (pm.response.code === 200) {
    const responseJson = pm.response.json();
    if (responseJson.token) {
        pm.environment.set('jwt_token', responseJson.token);
        pm.environment.set('user_id', responseJson.user.id);
        pm.environment.set('user_role', responseJson.user.role);
        pm.environment.set('user_branch_id', responseJson.user.branch_id);
        console.log('Token saved successfully!');
    }
}
```

## 📂 โครงสร้าง Collections

### 🔐 Authentication
- **Login** - เข้าสู่ระบบและเก็บ JWT token อัตโนมัติ
- **Get Profile** - ดูข้อมูลโปรไฟล์ตัวเอง
- **Change Password** - เปลี่ยนรหัสผ่าน

### 📚 Public Courses
- **Get All Courses** - ดูรายการคอร์สทั้งหมด (ไม่ต้อง login)
- **Get Single Course** - ดูรายละเอียดคอร์สเดียว

### 👥 User Management (Admin/Owner เท่านั้น)
- **Get Users** - ดูรายการผู้ใช้ (มี pagination และ filters)
- **Create User** - สร้างผู้ใช้ใหม่
- **Update User** - แก้ไขข้อมูลผู้ใช้
- **Upload User Avatar** - อัปโหลดรูปโปรไฟล์

### 🏢 Branch Management
- **Get Branches** - ดูรายการสาขา
- **Create Branch** - สร้างสาขาใหม่

### 🎓 Student Management
- **Get Students** - ดูรายการนักเรียน (มี filters ตาม age group, CEFR level)
- **Create Student Profile** - สร้างโปรไฟล์นักเรียนแบบครบถ้วน

### 👨‍🏫 Teacher Management
- **Get Teachers** - ดูรายการครู (filter ตาม type, specialization)
- **Create Teacher Profile** - สร้างโปรไฟล์ครูแบบครบถ้วน
- **Get Teacher Specializations** - ดูรายการความเชี่ยวชาญของครู

### 🏫 Room Management
- **Get Rooms** - ดูรายการห้องเรียน (filter ตาม capacity, equipment)
- **Create Room** - สร้างห้องเรียนใหม่
- **Update Room Status** - อัปเดตสถานะห้อง (available/occupied/maintenance)

### 📖 Course Management (Protected)
- **Create Course** - สร้างคอร์สใหม่ (Admin/Owner เท่านั้น)

### 🔔 Notifications
- **Get Notifications** - ดูการแจ้งเตือน (มี filters)
- **Create Notification** - สร้างการแจ้งเตือนใหม่ (Admin เท่านั้น)
- **Mark as Read** - ทำเครื่องหมายว่าอ่านแล้ว
- **Get Unread Count** - นับจำนวนที่ยังไม่ได้อ่าน

## 🔧 Environment Variables

| Variable | คำอธิบาย | ตัวอย่าง |
|----------|----------|----------|
| `base_url` | URL หลักของ API | `http://localhost:3000` |
| `jwt_token` | JWT Token (เก็บอัตโนมัติหลัง login) | `eyJhbGciOiJIUzI1NiIs...` |
| `user_id` | ID ของผู้ใช้ปัจจุบัน | `1` |
| `user_role` | Role ของผู้ใช้ | `admin`, `teacher`, `student` |
| `user_branch_id` | Branch ID ของผู้ใช้ | `1` |

## 📋 ตัวอย่างการใช้งาน

### 1. เริ่มต้น - Login
```
POST {{base_url}}/api/auth/login
{
  "username": "admin",
  "password": "password123"
}
```

### 2. ดูข้อมูลโปรไฟล์
```
GET {{base_url}}/api/profile
Authorization: Bearer {{jwt_token}}
```

### 3. สร้างนักเรียนใหม่
```
POST {{base_url}}/api/students
Authorization: Bearer {{jwt_token}}
{
  "user_id": 3,
  "first_name": "สมชาย",
  "last_name": "ใจดี",
  "age": 20,
  "age_group": "adults",
  "cefr_level": "A2"
}
```

## 🎯 Features พิเศษ

### ✅ Auto Token Management
- Login แล้วจะเก็บ token อัตโนมัติ
- ไม่ต้องคัดลอก token มาใส่เอง

### ✅ Global Scripts
- **Pre-request**: ตั้งค่า base_url อัตโนมัติ
- **Test**: แสดง response status และ error handling

### ✅ Rich Documentation
- ทุก request มี description อธิบายการใช้งาน
- ตัวอย่าง body และ parameters ครบถ้วน
- อธิบาย query parameters และ headers

### ✅ Error Handling
- แสดงข้อความแจ้งเตือนเมื่อ 401 (ต้อง login)
- แสดงข้อความเมื่อ 403 (ไม่มีสิทธิ์)
- Log response time และ status

## 🔧 การปรับแต่ง

### เปลี่ยน Base URL
1. ไปที่ Environment
2. แก้ไขค่า `base_url` เป็น server อื่น
3. เช่น: `https://api.englishkorat.com`

### ใช้งานกับ Production
```json
{
  "base_url": "https://your-production-api.com",
  "jwt_token": "",
  "user_id": "",
  "user_role": "",
  "user_branch_id": ""
}
```

## 🚨 หมายเหตุสำคัญ

1. **ต้อง Login ก่อน** - ส่วนใหญ่ของ endpoints ต้องการ authentication
2. **Role Permissions** - บาง endpoints ต้องการ role ระดับ admin/owner
3. **File Upload** - สำหรับ avatar upload ใช้ form-data
4. **Pagination** - หลาย endpoints รองรับ `page` และ `limit` parameters
5. **Bilingual** - API รองรับภาษาไทยและอังกฤษ

## 🎉 พร้อมใช้งาน!

1. Import ทั้ง Collection และ Environment
2. เลือก Environment ที่มุมขวาบน
3. เริ่มต้นด้วยการ Login
4. เริ่มใช้งาน APIs อื่น ๆ ได้เลย!

---
*Created for English Korat Learning Management System*
