# Password Reset Setup Guide

คู่มือการติดตั้งและใช้งานระบบ Password Reset สำหรับ English Korat API

## 📋 Overview

ระบบ Password Reset ที่พัฒนาขึ้นมีฟีเจอร์หลัก 3 อย่าง:
1. **Generate Reset Token** - Admin/Owner สร้าง token ให้ user
2. **Reset by Admin** - Admin/Owner reset password โดยตรง  
3. **Reset with Token** - User ใช้ token เพื่อ reset เอง

## 🔐 Security Features

### Role-based Access Control
```
Owner ──→ สามารถ reset ได้ทุกคน (Admin, Teacher, Student)
  │
Admin ──→ สามารถ reset ได้ (Admin, Teacher, Student) ❌ ยกเว้น Owner
  │  
Teacher ─→ ❌ ไม่สามารถ reset ให้คนอื่นได้
  │
Student ─→ ❌ ไม่สามารถ reset ให้คนอื่นได้
```

### Key Security Rules
- ✅ **Admin ไม่สามารถ reset password ของ Owner ได้**
- ✅ **Token มีอายุ 1 ชั่วโมง** หลังจากนั้นจะหมดอายุ
- ✅ **Password ใหม่ต้องมีอย่างน้อย 6 ตัวอักษร**
- ✅ **Token ใช้ได้ครั้งเดียว** หลังใช้แล้วจะถูกลบ
- ✅ **Cryptographically secure token** สร้างด้วย crypto/rand

## 🛠️ Installation

### 1. Database Migration

รันคำสั่ง SQL เพื่อเพิ่มฟิลด์ใหม่ในตาราง users:

```sql
-- Migration: Add password reset fields to users table
ALTER TABLE users 
ADD COLUMN password_reset_token VARCHAR(255) NULL,
ADD COLUMN password_reset_expires TIMESTAMP NULL,
ADD COLUMN password_reset_by_admin BOOLEAN DEFAULT FALSE;

-- Add index for faster token lookups
CREATE INDEX idx_users_password_reset_token ON users(password_reset_token);
CREATE INDEX idx_users_password_reset_expires ON users(password_reset_expires);
```

### 2. Code Changes

ฟีเจอร์ Password Reset ถูกเพิ่มในไฟล์ต่อไปนี้:

#### `models/models.go`
```go
// เพิ่มฟิลด์ใหม่ใน User struct
PasswordResetToken    string     `json:"-" gorm:"size:255"`
PasswordResetExpires  *time.Time `json:"-"`
PasswordResetByAdmin  bool       `json:"-" gorm:"default:false"`
```

#### `controllers/auth.go`
```go
// เพิ่ม methods ใหม่
- GeneratePasswordResetToken()
- ResetPasswordByAdmin()
- ResetPasswordWithToken()
```

#### `routes/routes.go`
```go
// เพิ่ม endpoints ใหม่
- POST /api/password-reset/generate-token
- POST /api/password-reset/reset-by-admin  
- POST /api/auth/reset-password-token
```

### 3. Build & Run

```bash
# Build application
go build

# Run application  
./englishkorat_go
# หรือ
go run main.go
```

## 📡 API Endpoints

### 1. Generate Password Reset Token
```http
POST /api/password-reset/generate-token
Authorization: Bearer {jwt_token}
Content-Type: application/json

{
  "user_id": 5
}
```

**Response:**
```json
{
  "message": "Password reset token generated successfully",
  "token": "a1b2c3d4e5f6...",
  "expires_at": "2025-09-09T15:30:00Z",
  "user": {
    "id": 5,
    "username": "john_doe",
    "email": "john@example.com"
  }
}
```

### 2. Reset Password by Admin
```http
POST /api/password-reset/reset-by-admin
Authorization: Bearer {jwt_token}
Content-Type: application/json

{
  "user_id": 5,
  "new_password": "newSecurePassword123",
  "require_password_change": true
}
```

### 3. Reset Password with Token
```http
POST /api/auth/reset-password-token
Content-Type: application/json

{
  "token": "a1b2c3d4e5f6...",
  "new_password": "userChosenPassword123"
}
```

## 🧪 Testing with Postman

### Import Collection และ Environment

1. **Import Collection:**
   - ไฟล์: `English_Korat_Password_Reset_API.postman_collection.json`

2. **Import Environment:**
   - ไฟล์: `English_Korat_Password_Reset_API.postman_environment.json`

3. **เลือก Environment:**
   - ที่มุมขวาบน เลือก "English Korat Password Reset - Development"

### Test Scenarios

#### Scenario 1: Token-based Reset (แนะนำ)
```
1. Login as Admin
2. Generate Reset Token สำหรับ user
3. Copy token จาก response  
4. User reset password ด้วย token
5. ✅ Success: User สามารถใช้รหัสผ่านใหม่ได้
```

#### Scenario 2: Direct Admin Reset (เร่งด่วน)
```
1. Login as Admin/Owner
2. Reset password โดยตรง
3. ✅ Success: User ใช้รหัสผ่านใหม่ได้ทันที
```

#### Scenario 3: Security Test (Admin vs Owner)
```
1. Login as Admin
2. Try to reset Owner password
3. ❌ Expected: 403 Forbidden
4. ✅ Success: Admin ไม่สามารถ reset Owner ได้
```

### Environment Variables

```json
{
  "base_url": "http://localhost:8080",
  "auth_token": "",              // Auto-saved from login
  "reset_token": "",             // Auto-saved from generate token
  "target_user_id": "3",         // ID ของ user ที่จะทดสอบ
  "admin_username": "admin",
  "admin_password": "password123",
  "owner_username": "owner", 
  "owner_password": "password123"
}
```

## 🔍 Troubleshooting

### Common Issues

#### 1. 403 Forbidden - "Admin cannot reset owner password"
```
Problem: Admin พยายาม reset password ของ Owner
Solution: ใช้ Owner account หรือเปลี่ยน target user
```

#### 2. 400 Bad Request - "Invalid or expired reset token"
```
Problem: Token หมดอายุหรือไม่ถูกต้อง
Solution: สร้าง token ใหม่ (อายุ 1 ชั่วโมง)
```

#### 3. 400 Bad Request - Password validation
```
Problem: Password สั้นกว่า 6 ตัวอักษร
Solution: ใช้รหัสผ่านที่ยาวขึ้น
```

#### 4. 401 Unauthorized
```
Problem: ไม่ได้ล็อกอินหรือ token หมดอายุ
Solution: Login ใหม่และตรวจสอบ auth_token
```

### Debug Steps

1. **ตรวจสอบ Database:**
   ```sql
   SELECT username, role, password_reset_token, password_reset_expires 
   FROM users 
   WHERE id = ?;
   ```

2. **ตรวจสอบ Logs:**
   ```
   Look for entries in logs/app.log:
   - "CREATE password_reset_token"
   - "UPDATE password_reset_admin"
   - "UPDATE password_reset_token"
   ```

3. **ตรวจสอบ Environment Variables:**
   ```
   Console log ใน Postman จะแสดง:
   - auth_token status
   - reset_token value
   - User role information
   ```

## 📋 Workflow Examples

### Production Workflow

#### 1. User ลืมรหัสผ่าน
```
1. User ติดต่อ Admin
2. Admin login และสร้าง reset token
3. Admin ส่ง token ให้ user (email, Line, etc.)
4. User ใช้ token reset password
5. User login ด้วยรหัสผ่านใหม่
```

#### 2. Emergency Reset  
```
1. User ต้องการเข้าใช้งานทันที
2. Admin/Owner reset password โดยตรง
3. แจ้งรหัสผ่านใหม่ให้ user
4. User login ทันที
5. (Optional) User เปลี่ยนรหัสผ่านใหม่
```

#### 3. Mass Password Reset
```
1. Owner login
2. Loop through users และ reset password
3. ส่งรหัสผ่านใหม่ให้แต่ละคน
4. Set require_password_change = true
5. Users จะต้องเปลี่ยนรหัสผ่านเมื่อล็อกอินครั้งต่อไป
```

## 🔒 Security Best Practices

### 1. Token Management
- ✅ สร้าง token ใหม่ทุกครั้ง
- ✅ ลบ token หลังใช้งาน
- ✅ ตรวจสอบหมดอายุก่อนใช้
- ✅ ใช้ secure random generation

### 2. Password Security
- ✅ Hash passwords ด้วย bcrypt
- ✅ Validate password strength
- ✅ Log ทุกการทำงาน
- ✅ Role-based permissions

### 3. Operational Security
- ✅ ส่ง token ผ่านช่องทางปลอดภัย
- ✅ ไม่เก็บ token ใน plain text logs
- ✅ Monitor reset activities
- ✅ Set appropriate token expiration

## 📊 Monitoring & Logging

### Log Entries ที่ควรติดตาม

```json
// Token Generation
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

// Admin Reset
{
  "action": "UPDATE", 
  "resource": "password_reset_admin",
  "user_id": 5,
  "details": {
    "target_user": "john_doe",
    "reset_by": "admin_user",
    "require_password_change": true
  }
}

// Token Usage
{
  "action": "UPDATE",
  "resource": "password_reset_token", 
  "user_id": 5,
  "details": {
    "username": "john_doe",
    "method": "token_reset"
  }
}
```

### Metrics ที่ควรเก็บ

- จำนวน token ที่สร้างต่อวัน
- จำนวน token ที่ใช้งานจริง
- จำนวน admin reset vs token reset
- อัตราการหมดอายุของ token
- Failed reset attempts

## ✅ Production Checklist

### Pre-deployment
- [ ] รัน database migration
- [ ] ทดสอบทุก endpoints ด้วย Postman
- [ ] ตรวจสอบ role permissions
- [ ] ทดสอบ token expiration
- [ ] ตรวจสอบ logging system

### Post-deployment  
- [ ] ทดสอบ reset workflow จริง
- [ ] ตรวจสอบ security restrictions
- [ ] Monitor log entries
- [ ] ทดสอบ email/notification integration (ถ้ามี)
- [ ] Setup monitoring alerts

---

## 🎉 Ready for Production!

ระบบ Password Reset พร้อมใช้งานแล้วพร้อมด้วย:
- ✅ Comprehensive security controls
- ✅ Role-based permissions  
- ✅ Token-based security
- ✅ Admin override capabilities
- ✅ Complete audit logging
- ✅ Production-ready error handling
