# English Korat Go API Documentation

## Quick Start Guide

### 1. Environment Setup

```bash
# Copy environment configuration
cp .env.example .env

# Edit .env file with your database credentials
# DB_HOST=localhost
# DB_PORT=3306
# DB_USER=root
# DB_PASSWORD=your_password
# DB_NAME=englishkorat_go
```

### 2. Database Setup

Make sure MySQL is running and create the database:
```sql
CREATE DATABASE englishkorat_go;
```

### 3. Start the Application

```bash
# Install dependencies
go mod tidy

# Build and run
make run

# Or run directly
go run main.go
```

### 4. Development with EC2 Tunnel

Use the PowerShell script for development with EC2:
```powershell
.\start-dev.ps1
```

## API Endpoints

### Base URL
```
http://localhost:3000/api
```

### Import Class Progress

- Method: POST
- Path: `/api/import/class-progress`
- Auth: JWT (Owner/Admin)
- Content-Type: `multipart/form-data`
- Form fields:
  - `file`: CSV or XLSX containing columns like: `FileName, FileId, SpreadsheetURL, SheetTab, Student1..4, StudentEN1..4, Level, CoursePath, TargetHours, SpeacialHours, TotalHour, Branch, No, LessonPlan, Date, Hour, WarmUp, Topic, LastPage, Teacher, Progress check, Comment, Goal + Infomation, Book`

Behavior:
- Creates or finds Course by `CoursePath` (fallback to `Level`) if not exists.
- Creates or finds Group named by `FileName/Level` (fallback to joined student names) if not exists, linked to the course.
- For each Student column pair (TH/EN), creates User if not exists with username = Thai nickname if available else English; default password = `1424123` (hashed), role = `student`.
- Creates Student profile if not exists and adds to the Group.
- Maps Teacher by nickname (Thai/English) if found; otherwise leaves null.
- Creates Book record by name if not exists and links it to the progress entry.
- Stores each row as a Class Progress record with session info.

Response:
```
{
  "success": true,
  "created": 21,
  "skipped": 2,
  "errors": ["row 12: ..."]
}
```

Notes:
- Date supports formats `DD/MM/YY`, `DD/MM/YYYY`, `YYYY-MM-DD`.
- Branch mapping takes the first number in `Branch` (e.g., "1,3"). If not present, defaults to Online branch (id=3).

### Public Endpoints (No Authentication Required)

#### Get All Courses
```http
GET /api/public/courses
```

Query Parameters:
- `branch_id` - Filter by branch ID
- `status` - Filter by status (default: active)
- `course_type` - Filter by course type
- `level` - Filter by level

Example Response:
```json
{
  "courses": [
    {
      "id": 47,
      "name": "TOEIC Foundation",
      "code": "TECH-TOEIC-FOUND",
      "course_type": "toeic_prep",
      "branch_id": 2,
      "description": "TOEIC preparation foundation",
      "status": "active",
      "level": "Foundation",
      "branch": {
        "id": 2,
        "name_en": "Branch 2 Technology Branch",
        "name_th": "สาขา 2 มหาวิทยาลัยเทคโนโลยีราชมงคลอีสาน"
      }
    }
  ],
  "total": 1
}
```

#### Get Single Course
```http
GET /api/public/courses/:id
```

### Authentication

#### Login
```http
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "password123"
}
```

Response:
```json
{
  "message": "Login successful",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "branch_id": 1
  }
}
```

### Protected Endpoints (Require Authentication)

All protected endpoints require the Authorization header:
```http
Authorization: Bearer <jwt_token>
```

#### User Profile
```http
GET /api/profile
```

#### Change Password
```http
PUT /api/profile/password
Content-Type: application/json

{
  "current_password": "oldpassword",
  "new_password": "newpassword"
}
```

### User Management (Admin/Owner Only)

#### Get Users
```http
GET /api/users?page=1&limit=10&role=student&branch_id=1
```

#### Create User
```http
POST /api/users
Content-Type: application/json

{
  "username": "new_user",
  "password": "password123",
  "email": "user@example.com",
  "role": "student",
  "branch_id": 1
}
```

#### Update User
```http
PUT /api/users/:id
Content-Type: application/json

{
  "email": "newemail@example.com",
  "phone": "0812345678"
}
```

#### Upload Avatar
```http
POST /api/users/:id/avatar
Content-Type: multipart/form-data

avatar: <file>
```

### Branch Management

#### Get Branches
```http
GET /api/branches?active=true&type=offline
```

#### Create Branch
```http
POST /api/branches
Content-Type: application/json

{
  "name_en": "New Branch",
  "name_th": "สาขาใหม่",
  "code": "NEW",
  "address": "123 Main St",
  "phone": "044-123456",
  "type": "offline"
}
```

### Student Management

#### Get Students
```http
GET /api/students?page=1&limit=10&age_group=adults&cefr_level=B1
```

#### Create Student Profile
```http
POST /api/students
Content-Type: application/json

{
  "user_id": 3,
  "first_name": "John",
  "last_name": "Doe",
  "age": 25,
  "age_group": "adults",
  "cefr_level": "B1"
}
```

### Teacher Management

#### Get Teachers
```http
GET /api/teachers?teacher_type=Both&active=true&branch_id=1
```

#### Create Teacher Profile
```http
POST /api/teachers
Content-Type: application/json

{
  "user_id": 8,
  "first_name": "Jane",
  "last_name": "Smith",
  "teacher_type": "Both",
  "specializations": "IELTS, TOEIC",
  "branch_id": 1
}
```

#### Get Teacher Specializations
```http
GET /api/teachers/specializations
```

### Room Management

#### Get Rooms
```http
GET /api/rooms?branch_id=1&status=available&min_capacity=8
```

#### Create Room
```http
POST /api/rooms
Content-Type: application/json

{
  "branch_id": 1,
  "room_name": "Room A4",
  "capacity": 10,
  "equipment": ["whiteboard", "projector", "air_conditioning"]
}
```

#### Update Room Status
```http
PATCH /api/rooms/:id/status
Content-Type: application/json

{
  "status": "occupied"
}
```

### Course Management (Protected)

#### Create Course
```http
POST /api/courses
Content-Type: application/json

{
  "name": "Advanced English",
  "code": "ADV-ENG-001",
  "course_type": "english_4skills",
  "branch_id": 1,
  "description": "Advanced English course",
  "level": "Advanced"
}
```

### Notification System

#### Get Notifications
```http
GET /api/notifications?page=1&limit=10&read=false&type=info
```

#### Create Notification (Admin Only)
```http
POST /api/notifications
Content-Type: application/json

{
  "role": "student",
  "title": "New Course Available",
  "title_th": "คอร์สใหม่มาแล้ว",
  "message": "We have a new IELTS course starting next month",
  "message_th": "เรามีคอร์ส IELTS ใหม่เริ่มเดือนหน้า",
  "type": "info"
}
```

#### Mark as Read
```http
PATCH /api/notifications/:id/read
```

#### Get Unread Count
```http
GET /api/notifications/unread-count
```

## Role-Based Access Control

### Roles:
- **Owner**: Full system access
- **Admin**: Full system access except owner management
- **Teacher**: Access to teaching-related features
- **Student**: Access to learning-related features

### Permission Matrix:

| Endpoint | Owner | Admin | Teacher | Student |
|----------|-------|-------|---------|---------|
| Public Courses | ✅ | ✅ | ✅ | ✅ |
| Profile | ✅ | ✅ | ✅ | ✅ |
| User Management | ✅ | ✅ | ❌ | ❌ |
| Branch Management | ✅ | ✅ | 👁️ | ❌ |
| Student Management | ✅ | ✅ | ✅ | ❌ |
| Teacher Management | ✅ | ✅ | 👁️ | ❌ |
| Room Management | ✅ | ✅ | ✅ | ❌ |
| Course Management | ✅ | ✅ | ❌ | ❌ |
| Notifications | ✅ | ✅ | ✅ | ✅ |

👁️ = Read-only access

## Database Models

### Core Models:
- **Branch**: Multi-branch support with bilingual names
- **User**: Authentication and user management
- **Student**: Comprehensive student profiles
- **Teacher**: Teacher profiles with specializations
- **Room**: Room management with equipment tracking
- **Course**: Course catalog
- **ActivityLog**: Complete audit trail
- **Notification**: Bilingual notification system

### Features:
- **Auto-migration**: Automatic database schema updates
- **Bilingual Support**: Thai/English fields throughout
- **JSON Fields**: Flexible data storage for complex structures
- **Soft Deletes**: Data recovery capability
- **Timestamps**: Automatic created/updated tracking

## Error Handling

All endpoints return consistent error responses:

```json
{
  "error": "Error description",
  "code": 400,
  "path": "/api/endpoint",
  "method": "POST"
}
```

Common HTTP Status Codes:
- `200` - Success
- `201` - Created
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `409` - Conflict
- `500` - Internal Server Error

## File Upload

### Avatar Upload:
- Supports: JPG, JPEG, PNG, GIF
- Auto-converts to WebP format
- Stores in S3 bucket with organized folder structure
- Maximum size: 10MB (configurable)

Folder structure: `avatars/{user_id}/{year}/{month}/{day}/{random_id}.webp`

## Logging

### Activity Logging:
- All CRUD operations are logged
- IP address and user agent tracking
- Detailed operation context
- Searchable and filterable

### Application Logging:
- Structured JSON logging
- Different log levels (info, warn, error)
- File and console output
- Request/response logging

## Development Tools

### Makefile Commands:
```bash
make build          # Build the application
make run            # Build and run
make dev            # Development mode with live reload
make test           # Run tests
make clean          # Clean build artifacts
make format         # Format code
make lint           # Lint code
make health         # Check API health
```

### Database Seeding:
The application automatically seeds initial data on first run:
- 3 branches (Mall, RMUTI, Online)
- Sample users with different roles
- Sample students and teachers
- Sample rooms and courses

## Security Features

- JWT-based authentication
- Password hashing with bcrypt
- CORS protection
- Security headers
- Input validation
- SQL injection prevention
- Role-based authorization
- Activity audit logging