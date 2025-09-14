# English Korat Go API - Group-Based Scheduling System Documentation

## Overview

This document provides comprehensive documentation for the English Korat Go API's new Group-based scheduling system. The system has been redesigned to support a more flexible workflow where:

- Students are organized into Groups
- Groups contain Courses and have payment status tracking
- Schedules are created for Groups (for classes) or Participants (for events/appointments)
- Sessions can have different teachers and rooms assigned per session
- Full confirmation and notification workflow is implemented

## Authentication

All protected endpoints require JWT authentication. Include the token in the Authorization header:

```
Authorization: Bearer <your_jwt_token>
```

### User Roles
- **Owner**: Full system access
- **Admin**: Administrative access, can manage schedules and groups
- **Teacher**: Can view and confirm assigned schedules, update session status
- **Student**: Can view their own schedules and participate in sessions

## Base URL
```
http://localhost:3000/api
```

## Core Workflow

### For Class Schedules (ตารางเรียน):
1. **Create Groups** with students and set payment status
2. **Create Class Schedule** linked to a Group
3. **Teacher confirms the schedule** (or admin/owner can confirm for anyone)
4. **Sessions are auto-generated** based on the recurring pattern
5. **Session confirmations** and notifications are handled
6. **Reminders** sent before confirmed sessions

### For Event/Appointment Schedules:
1. **Create Event/Appointment Schedule** with participant list
2. **Participants confirm** their attendance
3. **Sessions are created** for the event
4. **Notifications and reminders** work the same way

---

## Groups Management

Groups represent learning groups containing students with course and payment information.

### Create Group
**POST** `/api/groups`

**Permissions:** Admin, Owner

**Request Body:**
```json
{
  "group_name": "English A1 Morning",
  "course_id": 1,
  "level": "A1",
  "max_students": 8,
  "payment_status": "pending",
  "description": "Morning English class for beginners"
}
```

**Response:**
```json
{
  "message": "Group created successfully",
  "group": {
    "id": 1,
    "group_name": "English A1 Morning",
    "course_id": 1,
    "level": "A1",
    "max_students": 8,
    "status": "active",
    "payment_status": "pending",
    "description": "Morning English class for beginners",
    "course": {
      "id": 1,
      "name": "Basic English A1",
      "level": "A1"
    },
    "members": []
  }
}
```

### Get Groups
**GET** `/api/groups`

**Permissions:** Teacher, Admin, Owner

**Query Parameters:**
- `course_id` (optional): Filter by course ID
- `status` (optional): Filter by status (active, inactive, suspended, full, need-feeling, empty)
- `payment_status` (optional): Filter by payment status (pending, deposit_paid, fully_paid)

**Example:** `/api/groups?course_id=1&status=active`

### Get Specific Group
**GET** `/api/groups/{id}`

**Permissions:** Teacher, Admin, Owner

### Add Member to Group
**POST** `/api/groups/{id}/members`

**Permissions:** Admin, Owner

**Request Body:**
```json
{
  "student_id": 123,
  "payment_status": "deposit_paid"
}
```

### Remove Member from Group
**DELETE** `/api/groups/{id}/members/{student_id}`

**Permissions:** Admin, Owner

### Update Payment Status
**PATCH** `/api/groups/{id}/payment-status`

**Permissions:** Admin, Owner

**Request Body:**
```json
{
  "payment_status": "fully_paid",
  "student_id": 123  // Optional: update specific member, otherwise update group
}
```

---

## Schedule Management

### Create Schedule
**POST** `/api/schedules`

**Permissions:** Admin, Owner

#### For Class Schedules:
```json
{
  "schedule_name": "English A1 Morning Class",
  "schedule_type": "class",
  "group_id": 1,
  "recurring_pattern": "weekly",
  "total_hours": 40,
  "hours_per_session": 2,
  "session_per_week": 2,
  "start_date": "2024-01-15T00:00:00Z",
  "estimated_end_date": "2024-03-15T00:00:00Z",
  "default_teacher_id": 5,
  "default_room_id": 3,
  "auto_reschedule": true,
  "session_start_time": "09:00",
  "custom_recurring_days": [1, 3],  // Monday, Wednesday
  "notes": "Beginner English class"
}
```

#### For Event/Appointment Schedules:
```json
{
  "schedule_name": "Student Orientation",
  "schedule_type": "event",
  "participant_user_ids": [10, 11, 12, 5],
  "recurring_pattern": "daily",
  "total_hours": 3,
  "hours_per_session": 3,
  "session_per_week": 1,
  "start_date": "2024-01-20T00:00:00Z",
  "estimated_end_date": "2024-01-20T00:00:00Z",
  "session_start_time": "10:00",
  "notes": "New student orientation session"
}
```

**Response:**
```json
{
  "message": "Schedule created successfully",
  "schedule": {
    "id": 15,
    "schedule_name": "English A1 Morning Class",
    "schedule_type": "class",
    "group_id": 1,
    "status": "assigned",
    "group": {
      "id": 1,
      "group_name": "English A1 Morning",
      "course": {
        "name": "Basic English A1",
        "level": "A1"
      }
    }
  }
}
```

### Get All Schedules
**GET** `/api/schedules`

**Permissions:** Admin, Owner

### Get My Schedules
**GET** `/api/schedules/my`

**Permissions:** All authenticated users

Returns schedules relevant to the current user:
- **Teachers**: Schedules where they are assigned (default or session teacher)
- **Students**: Schedules of groups they belong to
- **Admin/Owner**: All schedules

### Confirm Schedule
**PATCH** `/api/schedules/{id}/confirm`

**Permissions:** 
- Assigned teacher (for class schedules)
- Participants (for event/appointment schedules)
- Admin/Owner (can confirm any schedule)

**Request Body:**
```json
{
  "status": "scheduled"
}
```

**Response:**
```json
{
  "message": "Schedule confirmed successfully"
}
```

---

## Session Management

### Get Schedule Sessions
**GET** `/api/schedules/{id}/sessions`

**Permissions:** Teacher, Admin, Owner

**Response:**
```json
{
  "sessions": [
    {
      "id": 45,
      "schedule_id": 15,
      "session_date": "2024-01-15T00:00:00Z",
      "start_time": "2024-01-15T09:00:00Z",
      "end_time": "2024-01-15T11:00:00Z",
      "session_number": 1,
      "week_number": 1,
      "status": "scheduled",
      "is_makeup": false,
      "assigned_teacher_id": 5,
      "room_id": 3,
      "assigned_teacher": {
        "id": 5,
        "username": "teacher_john"
      },
      "room": {
        "id": 3,
        "room_name": "Room A"
      }
    }
  ]
}
```

### Update Session Status
**PATCH** `/api/schedules/sessions/{id}/status`

**Permissions:** Assigned teacher, Admin, Owner

**Request Body:**
```json
{
  "status": "confirmed",  // scheduled, confirmed, pending, completed, cancelled, rescheduled, no-show
  "notes": "Student confirmed attendance"
}
```

Valid Status Values:
- `scheduled`: Default status when session is created
- `confirmed`: Teacher/participant confirmed attendance
- `pending`: Waiting for confirmation
- `completed`: Session finished successfully
- `cancelled`: Session cancelled
- `rescheduled`: Session moved to different time
- `no-show`: Participant didn't attend

### Create Makeup Session
**POST** `/api/schedules/sessions/makeup`

**Permissions:** Teacher, Admin, Owner

**Request Body:**
```json
{
  "original_session_id": 45,
  "new_session_date": "2024-01-22T00:00:00Z",
  "new_start_time": "14:00",
  "cancelling_reason": "Student was sick",
  "new_session_status": "cancelled"  // cancelled, rescheduled, no-show
}
```

---

## Comments System

### Add Comment
**POST** `/api/schedules/comments`

**Permissions:** All authenticated users

**Request Body:**
```json
{
  "schedule_id": 15,     // Either schedule_id OR session_id
  "session_id": null,    // Required: one of these must be provided
  "comment": "Student requested extra practice materials"
}
```

### Get Comments
**GET** `/api/schedules/comments`

**Query Parameters:**
- `schedule_id`: Get comments for a schedule
- `session_id`: Get comments for a session

**Example:** `/api/schedules/comments?schedule_id=15`

**Response:**
```json
{
  "comments": [
    {
      "id": 8,
      "schedule_id": 15,
      "session_id": null,
      "user_id": 5,
      "comment": "Student requested extra practice materials",
      "created_at": "2024-01-15T10:30:00Z",
      "user": {
        "id": 5,
        "username": "teacher_john"
      }
    }
  ]
}
```

---

## Calendar and Schedule Views

### Get Teachers' Schedules
**GET** `/api/schedules/teachers`

**Permissions:** Teacher, Admin, Owner

**Response:**
```json
{
  "schedules": [
    {
      "id": 15,
      "schedule_name": "English A1 Morning Class",
      "schedule_type": "class",
      "status": "scheduled",
      "start_date": "2024-01-15T00:00:00Z",
      "estimated_end_date": "2024-03-15T00:00:00Z",
      "group": {
        "group_name": "English A1 Morning",
        "course": {
          "name": "Basic English A1",
          "level": "A1"
        }
      },
      "default_teacher": {
        "id": 5,
        "username": "teacher_john"
      }
    }
  ],
  "message": "Basic schedule list - full calendar view to be implemented"
}
```

### Get Calendar View
**GET** `/api/schedules/calendar`

**Permissions:** Teacher, Admin, Owner

**Response:**
```json
{
  "sessions": [
    {
      "id": 45,
      "schedule_id": 15,
      "session_date": "2024-01-15T00:00:00Z",
      "start_time": "2024-01-15T09:00:00Z",
      "end_time": "2024-01-15T11:00:00Z",
      "status": "confirmed",
      "schedule": {
        "schedule_name": "English A1 Morning Class",
        "group": {
          "course": {
            "name": "Basic English A1"
          }
        }
      }
    }
  ],
  "message": "Basic session list - full calendar view to be implemented"
}
```

---

## Notification System

The system automatically sends notifications for:

1. **Schedule Assignment**: When a teacher is assigned to a schedule
2. **Schedule Confirmation**: When a schedule is confirmed by teacher
3. **Session Reminders**: Before confirmed sessions start (configurable timing)

### Notification Preferences
Each user can configure their notification preferences (this will be implemented):

- Enable/disable schedule reminders
- Set reminder timing (minutes before session)
- Number of reminders (1-3)
- Reminder intervals

---

## Data Models

### Group Model
```json
{
  "id": 1,
  "group_name": "English A1 Morning",
  "course_id": 1,
  "level": "A1",
  "max_students": 8,
  "status": "active",        // active, inactive, suspended, full, need-feeling, empty
  "payment_status": "pending", // pending, deposit_paid, fully_paid
  "description": "Morning English class for beginners",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

### GroupMember Model
```json
{
  "id": 1,
  "group_id": 1,
  "student_id": 123,
  "payment_status": "deposit_paid", // pending, deposit_paid, fully_paid
  "joined_at": "2024-01-01T00:00:00Z",
  "status": "active"  // active, inactive, suspended
}
```

### Schedule Model
```json
{
  "id": 15,
  "schedule_name": "English A1 Morning Class",
  "schedule_type": "class",    // class, meeting, event, holiday, appointment
  "group_id": 1,               // For class schedules
  "created_by_user_id": 2,     // Who created this schedule
  "recurring_pattern": "weekly", // daily, weekly, bi-weekly, monthly, yearly, custom
  "total_hours": 40,
  "hours_per_session": 2,
  "session_per_week": 2,
  "start_date": "2024-01-15T00:00:00Z",
  "estimated_end_date": "2024-03-15T00:00:00Z",
  "actual_end_date": null,
  "default_teacher_id": 5,     // Default teacher for sessions
  "default_room_id": 3,        // Default room for sessions
  "status": "scheduled",       // scheduled, paused, completed, cancelled, assigned
  "auto_reschedule": true,
  "notes": "Beginner English class",
  "admin_assigned": "admin_user"
}
```

### Session Model
```json
{
  "id": 45,
  "schedule_id": 15,
  "session_date": "2024-01-15T00:00:00Z",
  "start_time": "2024-01-15T09:00:00Z",
  "end_time": "2024-01-15T11:00:00Z",
  "session_number": 1,
  "week_number": 1,
  "status": "confirmed",        // scheduled, confirmed, pending, completed, cancelled, rescheduled, no-show
  "cancelling_reason": "",
  "is_makeup": false,
  "makeup_for_session_id": null,
  "notes": "",
  "assigned_teacher_id": 5,     // Can be different per session
  "room_id": 3,                 // Can be different per session
  "confirmed_at": "2024-01-15T08:30:00Z",
  "confirmed_by_user_id": 5
}
```

### ScheduleParticipant Model (for events/appointments)
```json
{
  "id": 1,
  "schedule_id": 15,
  "user_id": 10,
  "role": "participant",    // organizer, participant, observer
  "status": "confirmed"     // invited, confirmed, declined, tentative
}
```

---

## Error Responses

All error responses follow this format:

```json
{
  "error": "Error message describing what went wrong",
  "code": 400,
  "path": "/api/schedules",
  "method": "POST"
}
```

### Common HTTP Status Codes:
- **200**: Success
- **201**: Created successfully
- **400**: Bad Request (validation error, missing required fields)
- **401**: Unauthorized (invalid or missing token)
- **403**: Forbidden (insufficient permissions)
- **404**: Not Found (resource doesn't exist)
- **409**: Conflict (e.g., student already in group)
- **500**: Internal Server Error

---

## Implementation Status

### ✅ Completed Features:
- Group management with payment status
- Schedule creation for both classes and events
- Session generation and management
- Comment system for schedules and sessions
- Basic notification system
- Permission-based access control
- Session confirmation workflow

### 🔄 In Progress / To Be Enhanced:
- Full calendar view with date filtering
- Advanced session reminders with user preferences
- Conflict detection for rooms and teachers
- Comprehensive reporting and analytics
- Bulk operations for group management

### 📋 Usage Examples:

#### Complete Class Schedule Workflow:

1. **Create a Group:**
```bash
POST /api/groups
{
  "group_name": "Advanced English B2",
  "course_id": 5,
  "level": "B2",
  "max_students": 6,
  "payment_status": "pending"
}
```

2. **Add Students to Group:**
```bash
POST /api/groups/1/members
{
  "student_id": 100,
  "payment_status": "deposit_paid"
}
```

3. **Create Schedule for Group:**
```bash
POST /api/schedules
{
  "schedule_name": "Advanced English B2 Evening",
  "schedule_type": "class", 
  "group_id": 1,
  "default_teacher_id": 8,
  "default_room_id": 5,
  "recurring_pattern": "weekly",
  "total_hours": 60,
  "hours_per_session": 3,
  "session_per_week": 2,
  "start_date": "2024-02-01T00:00:00Z",
  "estimated_end_date": "2024-04-01T00:00:00Z",
  "session_start_time": "18:00",
  "custom_recurring_days": [2, 4]
}
```

4. **Teacher Confirms Schedule:**
```bash
PATCH /api/schedules/15/confirm
{
  "status": "scheduled"
}
```

5. **Teacher Confirms Individual Sessions:**
```bash
PATCH /api/schedules/sessions/45/status
{
  "status": "confirmed",
  "notes": "All students confirmed attendance"
}
```

This comprehensive system provides full flexibility for managing both structured class schedules and ad-hoc events/appointments while maintaining proper permissions, payment tracking, and notification workflows.