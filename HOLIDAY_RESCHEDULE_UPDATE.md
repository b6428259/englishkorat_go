# Holiday Impact and Rescheduling Update

## Overview
Updated the schedule preview endpoint to provide detailed holiday information and improved rescheduling logic.

## Changes Made

### 1. Holiday Information Enhancement

#### Added Holiday Names to Response
- **New Field**: `holiday_name` in `HolidayImpact` struct
- The API now fetches holiday names (SUMMARY field) from MyHora API
- Holiday names are displayed in Thai (e.g., "วันสงกรานต์ (ครม.)")

#### Example Response:
```json
{
  "holiday_impacts": [
    {
      "session_number": 2,
      "date": "2025-10-13",
      "holiday_name": "วันสงกรานต์ (ครม.)",
      "shifted_to": "2025-10-14",
      "was_rescheduled": true
    }
  ]
}
```

### 2. Improved Rescheduling Logic

#### Previous Behavior (Incorrect)
- When a session fell on a holiday, it would be moved to the next available day
- This caused sessions to be shifted forward incrementally
- Example: If Monday was a holiday, Monday's session → Tuesday, Tuesday's session → Wednesday, etc.

#### New Behavior (Correct)
- Sessions on holidays are **removed from their original positions**
- Removed sessions are **appended to the end** of the schedule
- Sessions find the next available non-holiday date after the last scheduled session
- This preserves the original session order for non-holiday sessions

#### Example:
**Original Schedule:**
- Week 1: Monday (Session 1), Wednesday (Session 2)
- Week 2: Monday (Session 3 - **HOLIDAY**), Wednesday (Session 4)
- Week 3: Monday (Session 5), Wednesday (Session 6)

**New Schedule with Rescheduling:**
- Week 1: Monday (Session 1), Wednesday (Session 2)
- Week 2: Wednesday (Session 3) *[Renumbered from 4]*
- Week 3: Monday (Session 4), Wednesday (Session 5) *[Renumbered from 5, 6]*
- Week 3: **Friday (Session 6)** *[Holiday session appended and renumbered]*

**Key Point:** Session numbers are reindexed sequentially (1, 2, 3...) after rescheduling, so there are no gaps in the sequence.

### 3. New Service Functions

#### `GetThaiHolidaysWithNames(startYear, endYear int) (map[string]string, error)`
- Returns a map of date strings to holiday names
- Combines data from both JSON and ICS endpoints
- Provides fallback mechanisms for reliability

#### `fetchMyHoraJSONWithNames(year int) (map[string]string, error)`
- Fetches holidays from JSON endpoint with names
- Returns map: `{"2025-10-13": "วันสงกรานต์ (ครม.)"}`

#### `fetchMyHoraICSWithNames(year int) (map[string]string, error)`
- Fallback ICS parser that extracts both DTSTART and SUMMARY fields
- Handles ICS format parsing line by line
- Matches date with corresponding holiday name

### 4. Updated Data Structures

#### HolidayResponse (services/schedule_service.go)
```go
type HolidayResponse struct {
    VCALENDAR []struct {
        VEVENT []struct {
            DTStart      string `json:"DTSTART"`
            DTStartValue string `json:"DTSTART;VALUE=DATE,omitempty"`
            Summary      string `json:"SUMMARY"`
        } `json:"VEVENT"`
    } `json:"VCALENDAR"`
}
```

#### HolidayImpact (controllers/schedules.go)
```go
type HolidayImpact struct {
    SessionNumber  int    `json:"session_number"`
    Date           string `json:"date"`
    HolidayName    string `json:"holiday_name,omitempty"`  // NEW
    ShiftedTo      string `json:"shifted_to,omitempty"`
    WasRescheduled bool   `json:"was_rescheduled"`
}
```

## Technical Implementation

### RescheduleSessions Function
```go
func RescheduleSessions(sessions []models.Schedule_Sessions, holidays []time.Time) []models.Schedule_Sessions {
    // 1. Separate holiday and non-holiday sessions
    var rescheduledSessions []models.Schedule_Sessions
    var postponedSessions []models.Schedule_Sessions
    
    // 2. Collect sessions that fall on holidays
    for _, session := range sessions {
        if isHoliday(session.Date) {
            postponedSessions = append(postponedSessions, session)
        } else {
            rescheduledSessions = append(rescheduledSessions, session)
        }
    }
    
    // 3. Append postponed sessions to the end
    // Find next available dates after the last session
    for _, session := range postponedSessions {
        newDate := findNextNonHoliday(lastDate)
        session.Date = newDate
        session.Notes = "Rescheduled due to holiday"
        rescheduledSessions = append(rescheduledSessions, session)
        lastDate = newDate
    }
    
    // 4. Reindex session numbers sequentially
    for i := range rescheduledSessions {
        rescheduledSessions[i].Session_number = i + 1
    }
    
    return rescheduledSessions
}
```

### Controller Integration
```go
// Fetch holidays with names
holidayNames, err := services.GetThaiHolidaysWithNames(startYear, endYear)

// Build rescheduled session mapping
rescheduledMap := make(map[int]string)
for _, session := range generatedSessions {
    rescheduledMap[session.Session_number] = session.Date
}

// Create holiday impacts with names
for _, session := range originalSessions {
    if holidayName, ok := holidayNames[session.Date]; ok {
        impact := HolidayImpact{
            SessionNumber:  session.Session_number,
            Date:           session.Date,
            HolidayName:    holidayName,  // Include holiday name
            ShiftedTo:      rescheduledMap[session.Session_number],
            WasRescheduled: autoReschedule,
        }
        holidayImpacts = append(holidayImpacts, impact)
    }
}
```

## Benefits

1. **Better User Experience**: Users can now see which specific holiday caused the rescheduling
2. **Accurate Rescheduling**: Sessions maintain their logical order and are properly deferred
3. **Robust Fallback**: Multiple data sources (JSON + ICS) ensure reliability
4. **Clear Communication**: Holiday names in Thai make it easier for local users to understand

## Testing

### Endpoint
`POST http://localhost:3000/api/schedules/preview`

### Test Request
```json
{
  "schedule_name": "Test Schedule",
  "schedule_type": "class",
  "group_id": 1,
  "start_date": "2025-10-06",
  "estimated_end_date": "2025-11-03",
  "total_hours": 5,
  "hours_per_session": 1,
  "session_per_week": 1,
  "recurring_pattern": "weekly",
  "session_start_time": "09:01",
  "auto_reschedule": true
}
```

### Expected Response
- `holiday_impacts` array with `holiday_name` field populated
- Sessions properly rescheduled to end of schedule
- Original sessions preserved in `original_sessions` array for comparison

## Files Modified

1. **services/schedule_service.go**
   - Added `GetThaiHolidaysWithNames()`
   - Added `fetchMyHoraJSONWithNames()`
   - Added `fetchMyHoraICSWithNames()`
   - Modified `RescheduleSessions()` to append instead of shift
   - Updated `extractHolidaysFromResponse()` to return map with names

2. **controllers/schedules.go**
   - Updated `HolidayImpact` struct with `holiday_name` field
   - Modified `PreviewSchedule()` to use new holiday API
   - Updated session rescheduling logic to map by session number

## Backward Compatibility

- Existing code continues to work as `GetThaiHolidays()` is still available
- New `holiday_name` field is optional (uses `omitempty` tag)
- Clients not expecting the field will simply ignore it
