# Password Reset API Documentation

## Overview
ระบบ Reset Password สำหรับ English Korat API ที่รองรับการรีเซ็ตรหัสผ่านผ่าน 2 วิธี:
1. **Token-based Reset**: ใช้ token สำหรับการรีเซ็ตแบบปลอดภัย  
2. **Admin Reset**: Admin/Owner สามารถรีเซ็ตรหัสผ่านได้โดยตรง

## Security Rules
- **Admin ไม่สามารถ reset password ของ Owner ได้**
- **Owner สามารถ reset password ของทุกคนได้** (รวม Admin)
- **Token มีอายุ 1 ชั่วโมง** หลังจากนั้นจะหมดอายุ
- **Password ใหม่ต้องมีอย่างน้อย 6 ตัวอักษร**

---

## 1. Generate Password Reset Token

สร้าง token สำหรับการรีเซ็ตรหัสผ่าน (Admin/Owner เท่านั้น)

### Endpoint
```http
POST /api/password-reset/generate-token
Authorization: Bearer {jwt_token}
Content-Type: application/json
```

### Request Body
```json
{
  "user_id": 5
}
```

### Permissions
- **Owner**: สามารถสร้าง token ให้ทุกคนได้
- **Admin**: สามารถสร้าง token ให้ Admin, Teacher, Student ได้ (ยกเว้น Owner)

### Response (Success - 200)
```json
{
  "message": "Password reset token generated successfully",
  "token": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
  "expires_at": "2025-09-09T15:30:00Z",
  "user": {
    "id": 5,
    "username": "john_doe",
    "email": "john@example.com"
  }
}
```

### Response (Error - 403)
```json
{
  "error": "Admin cannot reset owner password"
}
```

### Use Cases
- Admin ต้องการให้ user รีเซ็ตรหัสผ่านเอง
- ส่ง token ผ่าน email หรือช่องทางอื่น
- ระบบรักษาความปลอดภัยสูง

---

## 2. Reset Password by Admin

Admin/Owner รีเซ็ตรหัสผ่านให้ user โดยตรง

### Endpoint
```http
POST /api/password-reset/reset-by-admin
Authorization: Bearer {jwt_token}
Content-Type: application/json
```

### Request Body
```json
{
  "user_id": 5,
  "new_password": "newSecurePassword123",
  "require_password_change": true
}
```

### Parameters
- `user_id` (required): ID ของ user ที่ต้องการรีเซ็ต
- `new_password` (required): รหัสผ่านใหม่ (อย่างน้อย 6 ตัวอักษร)
- `require_password_change` (optional): บังคับให้เปลี่ยนรหัสผ่านในครั้งต่อไป

### Permissions
- **Owner**: สามารถรีเซ็ต password ของทุกคนได้
- **Admin**: สามารถรีเซ็ต password ของ Admin, Teacher, Student ได้ (ยกเว้น Owner)

### Response (Success - 200)
```json
{
  "message": "Password reset successfully",
  "user": {
    "id": 5,
    "username": "john_doe",
    "email": "john@example.com"
  },
  "require_password_change": true
}
```

### Response (Error - 403)
```json
{
  "error": "Admin cannot reset owner password"
}
```

### Use Cases
- รีเซ็ตรหัสผ่านเร่งด่วน
- User ลืมรหัสผ่านและต้องการเข้าใช้ทันที
- ตั้งรหัสผ่านชั่วคราว

---

## 3. Reset Password with Token

User ใช้ token เพื่อรีเซ็ตรหัสผ่านเอง (Public endpoint)

### Endpoint
```http
POST /api/auth/reset-password-token
Content-Type: application/json
```

### Request Body
```json
{
  "token": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
  "new_password": "myNewSecurePassword123"
}
```

### Parameters
- `token` (required): Token ที่ได้รับจาก admin
- `new_password` (required): รหัสผ่านใหม่ (อย่างน้อย 6 ตัวอักษร)

### Response (Success - 200)
```json
{
  "message": "Password reset successfully"
}
```

### Response (Error - 400)
```json
{
  "error": "Invalid or expired reset token"
}
```

### Token Validation
- ✅ Token ต้องตรงกับที่เก็บไว้ในฐานข้อมูล
- ✅ Token ต้องยังไม่หมดอายุ (1 ชั่วโมง)
- ✅ หลังใช้แล้ว token จะถูกลบทันที

---

## Security Features

### 1. Role-based Access Control
```
Owner ──→ สามารถรีเซ็ตได้ทุกคน (Admin, Teacher, Student)
  │
Admin ──→ สามารถรีเซ็ตได้ (Admin, Teacher, Student) ❌ ไม่รวม Owner
  │  
Teacher ─→ ❌ ไม่สามารถรีเซ็ตให้คนอื่นได้
  │
Student ─→ ❌ ไม่สามารถรีเซ็ตให้คนอื่นได้
```

### 2. Token Security
- **Cryptographically Secure**: ใช้ `crypto/rand` สร้าง token
- **32 bytes = 256 bits**: ความยาว token ที่ปลอดภัย
- **Hex Encoding**: แปลงเป็น string ความยาว 64 ตัวอักษร
- **Single Use**: ใช้แล้วหมดอายุทันที

### 3. Password Requirements
- **Minimum Length**: 6 ตัวอักษร
- **Hashing**: ใช้ bcrypt สำหรับเข้ารหัส
- **No Plaintext Storage**: ไม่เก็บรหัสผ่านแบบ plaintext

### 4. Audit Logging
ทุกการทำงานจะถูกบันทึก log:
```json
{
  "action": "CREATE",
  "resource": "password_reset_token",
  "user_id": 5,
  "details": {
    "target_user": "john_doe",
    "generated_by": "admin_user",
    "expires_at": "2025-09-09T15:30:00Z"
  }
}
```

---

## Error Codes

| Code | Message | Description |
|------|---------|-------------|
| 400 | Invalid request body | Request body ไม่ถูกต้อง |
| 401 | User not found | ไม่ได้ล็อกอิน หรือ token หมดอายุ |
| 403 | Only admin and owner can... | ไม่มีสิทธิ์ |
| 403 | Admin cannot reset owner password | Admin ไม่สามารถรีเซ็ต Owner |
| 404 | User not found | ไม่พบ user ที่ต้องการรีเซ็ต |
| 400 | Invalid or expired reset token | Token ไม่ถูกต้องหรือหมดอายุ |
| 500 | Failed to generate reset token | ปัญหาเซิร์ฟเวอร์ |

---

## Workflow Examples

### Scenario 1: Admin สร้าง token ให้ Teacher
```
1. Admin: POST /api/password-reset/generate-token
   Body: {"user_id": 3}
   
2. System: ส่ง token กลับมา
   Response: {"token": "abc123...", "expires_at": "..."}
   
3. Admin: ส่ง token ให้ Teacher (email, Line, etc.)

4. Teacher: POST /api/auth/reset-password-token
   Body: {"token": "abc123...", "new_password": "newPass123"}
   
5. System: รีเซ็ตรหัสผ่านสำเร็จ
```

### Scenario 2: Owner รีเซ็ตรหัสผ่าน Admin โดยตรง
```
1. Owner: POST /api/password-reset/reset-by-admin
   Body: {
     "user_id": 2,
     "new_password": "tempPassword123",
     "require_password_change": true
   }
   
2. System: รีเซ็ตรหัสผ่านทันที
   Response: {"message": "Password reset successfully"}
   
3. Admin: ล็อกอินด้วยรหัสผ่านใหม่
```

### Scenario 3: Admin พยายามรีเซ็ต Owner (ถูกปฏิเสธ)
```
1. Admin: POST /api/password-reset/reset-by-admin
   Body: {"user_id": 1, "new_password": "hack123"}
   
2. System: ปฏิเสธ
   Response: {"error": "Admin cannot reset owner password"}
```

---

## Testing with Postman

ตัวอย่าง collection สำหรับทดสอบ:

```json
{
  "name": "Password Reset API",
  "requests": [
    {
      "name": "1. Login as Admin",
      "method": "POST",
      "url": "{{base_url}}/api/auth/login",
      "body": {
        "username": "admin",
        "password": "password123"
      }
    },
    {
      "name": "2. Generate Reset Token",
      "method": "POST", 
      "url": "{{base_url}}/api/password-reset/generate-token",
      "headers": {
        "Authorization": "Bearer {{auth_token}}"
      },
      "body": {
        "user_id": 5
      }
    },
    {
      "name": "3. Reset Password by Admin",
      "method": "POST",
      "url": "{{base_url}}/api/password-reset/reset-by-admin", 
      "headers": {
        "Authorization": "Bearer {{auth_token}}"
      },
      "body": {
        "user_id": 5,
        "new_password": "newSecurePass123",
        "require_password_change": true
      }
    },
    {
      "name": "4. Reset with Token (Public)",
      "method": "POST",
      "url": "{{base_url}}/api/auth/reset-password-token",
      "body": {
        "token": "{{reset_token}}",
        "new_password": "userChosenPassword123"
      }
    }
  ]
}
```

---

## Database Schema

### User Table เพิ่มฟิลด์ใหม่:
```sql
ALTER TABLE users 
ADD COLUMN password_reset_token VARCHAR(255) NULL,
ADD COLUMN password_reset_expires TIMESTAMP NULL,
ADD COLUMN password_reset_by_admin BOOLEAN DEFAULT FALSE;

CREATE INDEX idx_users_password_reset_token ON users(password_reset_token);
CREATE INDEX idx_users_password_reset_expires ON users(password_reset_expires);
```

### ฟิลด์ใหม่:
- `password_reset_token`: เก็บ token สำหรับรีเซ็ต
- `password_reset_expires`: วันหมดอายุของ token  
- `password_reset_by_admin`: flag บอกว่าถูกรีเซ็ตโดย admin

---

## Best Practices

### 1. Token Management
- ✅ สร้าง token ใหม่ทุกครั้ง
- ✅ ลบ token หลังใช้งาน
- ✅ ตรวจสอบหมดอายุก่อนใช้
- ✅ ใช้ secure random generation

### 2. Security
- ✅ Hash passwords ด้วย bcrypt
- ✅ Validate password strength
- ✅ Log ทุกการทำงาน
- ✅ Role-based permissions

### 3. User Experience  
- ✅ ข้อความ error ที่ชัดเจน
- ✅ Response time ที่เร็ว
- ✅ Token ที่ใช้งานง่าย
- ✅ Optional password change requirement

---

## 🔐 Ready to Use!

ระบบ Password Reset พร้อมใช้งานแล้วพร้อมด้วย:
- ✅ Security controls ที่เข้มงวด  
- ✅ Role-based permissions
- ✅ Comprehensive logging
- ✅ Token-based security
- ✅ Admin override capabilities
