# Student Registration Workflow API Documentation

## Overview

The redesigned student registration system supports a sophisticated workflow with status tracking and structured data handling. The system handles two types of registration (quick/full) and provides admin management endpoints for processing registrations through different stages.

## Registration Flow

### Status Transitions
1. **pending_review** → Student submits registration, awaiting admin review
2. **schedule_exam** → Admin schedules placement exam
3. **waiting_for_group** → Exam completed, waiting for group assignment
4. **active** → Student assigned to group and actively learning

## API Endpoints

### Public Registration

#### POST `/api/students/new-register`
**Description**: New structured registration endpoint with nested payload support

**Request Body Structure**:
```json
{
  "registration_type": "quick|full",
  "basic_information": {
    "first_name": "ชื่อ",
    "last_name": "นามสกุล", 
    "nickname_th": "ชื่อเล่นไทย",
    "nickname_en": "NicknameEN",
    "date_of_birth": "2000-01-01",
    "gender": "male|female|other"
  },
  "contact_information": {
    "phone": "0812345678",
    "email": "optional@email.com",
    "line_id": "lineid123",
    "address": "optional address"
  },
  "full_information": {
    // Required only when registration_type = "full"
    "citizen_id": "1234567890123",
    "first_name_en": "FirstName",
    "last_name_en": "LastName",
    "current_education": "University",
    "preferred_branch": 1,
    "preferred_language": "english|chinese",
    "language_level": "Beginner",
    "learning_style": "private|pair|group",
    "recent_cefr": "A1",
    "selected_courses": [1, 2, 3],
    "learning_goals": "Business English",
    "teacher_type": "Native",
    "preferred_time_slots": [...],
    "unavailable_time_slots": [...],
    "emergency_contact": "Emergency Contact Name",
    "emergency_phone": "0987654321"
  }
}
```

**Response**:
```json
{
  "success": true,
  "message": "ลงทะเบียนสำเร็จ! ทางทีมงานจะติดต่อกลับภายใน 24 ชั่วโมง",
  "data": {
    "student": {
      "id": 123,
      "first_name": "ชื่อ",
      "last_name": "นามสกุล",
      "nickname_th": "ชื่อเล่นไทย",
      "nickname_en": "NicknameEN",
      "registration_type": "full",
      "registration_status": "pending_review",
      "created_at": "2025-01-24T10:00:00Z"
    },
    "registration_id": "REG-2025-000123"
  }
}
```

### Admin Management Endpoints

#### PATCH `/api/students/:id`
**Description**: Complete or update student information
**Authorization**: Teacher or above

**Request Body**:
```json
{
  "first_name_en": "UpdatedFirstName",
  "last_name_en": "UpdatedLastName",
  "citizen_id": "1234567890123",
  "current_education": "University",
  "preferred_branch": 2,
  "preferred_language": "english",
  "language_level": "Intermediate",
  "learning_style": "group",
  "recent_cefr": "B1",
  "selected_courses": [1, 3, 5],
  "learning_goals": "Updated goals",
  "teacher_type": "Native",
  "preferred_time_slots": [...],
  "unavailable_time_slots": [...],
  "emergency_contact": "Emergency Name",
  "emergency_phone": "0987654321",
  "registration_status": "schedule_exam"
}
```

#### GET `/api/students/by-status/:status`
**Description**: Filter students by registration status
**Authorization**: Teacher or above

**Parameters**:
- `status`: `pending_review|schedule_exam|waiting_for_group|active`
- Query params: `page`, `limit` (pagination)

**Response**:
```json
{
  "students": [...],
  "total": 25,
  "page": 1,
  "limit": 10,
  "total_pages": 3,
  "status": "pending_review"
}
```

#### POST `/api/students/:id/exam-scores`
**Description**: Record placement exam scores
**Authorization**: Teacher or above

**Request Body**:
```json
{
  "grammar_score": 85,
  "speaking_score": 78,
  "listening_score": 82,
  "reading_score": 88,
  "writing_score": 80
}
```

**Response**:
```json
{
  "success": true,
  "message": "บันทึกคะแนนสอบสำเร็จ",
  "data": {
    "student_id": 123,
    "grammar_score": 85,
    "speaking_score": 78,
    "listening_score": 82,
    "reading_score": 88,
    "writing_score": 80,
    "average_score": 82.6
  }
}
```

## Validation Rules

### Basic Information
- `first_name`, `last_name`, `nickname_th`, `nickname_en`: Required, non-empty
- `date_of_birth`: Required, format YYYY-MM-DD, age 3-100
- `gender`: Required, must be `male|female|other`

### Contact Information  
- `phone`: Required, valid Thai phone number format
- `line_id`: Required, non-empty
- `email`: Optional, valid email format if provided
- `address`: Optional

### Full Information (Required for `registration_type: "full"`)
- `citizen_id`: Required, exactly 13 digits
- `current_education`: Required, non-empty
- `preferred_branch`: Required, valid branch ID
- `preferred_language`: Required, `english|chinese`
- `language_level`: Required, non-empty
- `learning_style`: Required, `private|pair|group`
- `recent_cefr`: Required, non-empty
- `teacher_type`: Required, non-empty

### Exam Scores
- All scores: Required, integer 0-100
- Scores: `grammar_score`, `speaking_score`, `listening_score`, `reading_score`, `writing_score`

## Registration Types

### Quick Registration
- Minimal information required
- Only basic and contact information needed
- Status starts at `pending_review`
- Admin must complete full information later

### Full Registration  
- Complete information provided upfront
- All sections required (basic, contact, full)
- Ready for immediate processing
- Status starts at `pending_review`

## Database Schema Updates

### Student Model Fields Added
```go
// Registration Status Management
RegistrationStatus   string `json:"registration_status" gorm:"type:enum('pending_review','schedule_exam','waiting_for_group','active');default:'pending_review'"`
RegistrationType     string `json:"registration_type" gorm:"type:enum('quick','full');default:'full'"`

// Contact Information
Phone                string `json:"phone" gorm:"size:20"`
Email                string `json:"email" gorm:"size:255"`
LineID               string `json:"line_id" gorm:"size:100"`

// Test Scores (nullable until exam completed)
GrammarScore         *int   `json:"grammar_score"`
SpeakingScore        *int   `json:"speaking_score"`
ListeningScore       *int   `json:"listening_score"`
ReadingScore         *int   `json:"reading_score"`
WritingScore         *int   `json:"writing_score"`

// Preferred Branch Relationship
PreferredBranchID    *uint  `json:"preferred_branch_id" gorm:"index"`
PreferredBranch      Branch `json:"preferred_branch,omitempty" gorm:"foreignKey:PreferredBranchID"`
```

## Error Handling

### Common Error Responses
```json
{
  "success": false,
  "message": "ข้อมูลไม่ถูกต้อง",
  "errors": [
    "ชื่อ (ไทย) จำเป็นต้องกรอก",
    "เบอร์โทรศัพท์ไม่ถูกต้อง"
  ]
}
```

### Status Codes
- `200`: Success
- `400`: Bad Request (validation errors)
- `401`: Unauthorized
- `403`: Forbidden 
- `404`: Not Found
- `500`: Internal Server Error

## Legacy Support

The system maintains backward compatibility with the existing registration endpoint:
- `POST /api/students/student-register` - Original registration endpoint
- All existing functionality preserved
- No breaking changes to current integrations

## Usage Examples

### Quick Registration Flow
1. User submits minimal information via `/api/students/new-register`
2. Admin reviews via `GET /api/students/by-status/pending_review`
3. Admin completes info via `PATCH /api/students/:id`
4. Admin schedules exam (status → `schedule_exam`)
5. Admin records scores via `POST /api/students/:id/exam-scores`
6. Admin assigns to group (status → `waiting_for_group` → `active`)

### Full Registration Flow
1. User submits complete information via `/api/students/new-register`
2. Admin reviews via `GET /api/students/by-status/pending_review`
3. Admin schedules exam (status → `schedule_exam`) 
4. Admin records scores via `POST /api/students/:id/exam-scores`
5. Admin assigns to group (status → `waiting_for_group` → `active`)

## Security Notes

- All admin endpoints require authentication (JWT token)
- Role-based access control enforced
- Input validation on all endpoints
- SQL injection protection via GORM
- Phone number and citizen ID format validation
