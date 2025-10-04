# Session Renumbering Fix for Preview and Create Schedule

## Problem
After implementing the lift-and-append holiday rescheduling logic, there was an issue with tracking which sessions moved to which dates in the `PreviewSchedule` endpoint. The problem occurred because:

1. `RescheduleSessions()` now reindexes session numbers (1, 2, 3...)
2. The preview was trying to map rescheduled sessions using `session_number`
3. After reindexing, the session numbers changed, breaking the mapping

## Solution

### 1. Updated `RescheduleSessions()` in `services/schedule_service.go`
Added session number reindexing at the end:
```go
// Reindex session numbers เรียงลำดับใหม่ 1, 2, 3...
for i := range rescheduledSessions {
    rescheduledSessions[i].Session_number = i + 1
}
```

### 2. Updated `PreviewSchedule()` in `controllers/schedules.go`
Changed from mapping by `session_number` to mapping by `date`:

**Before:**
```go
// Build mapping from original session number to rescheduled session
rescheduledMap := make(map[int]string)
for _, session := range generatedSessions {
    rescheduledMap[session.Session_number] = formatSessionDate(session.Session_date)
}
```

**After:**
```go
// Build mapping from original date to rescheduled date BEFORE reindexing
originalToRescheduledDate := make(map[string]string)

// Track which dates moved where
// ... (detailed mapping logic)
```

## How It Works

### Step-by-Step Process:

1. **Generate original sessions**
   ```
   Session 1: 2025-10-06 (Monday)
   Session 2: 2025-10-13 (Monday) - HOLIDAY
   Session 3: 2025-10-20 (Monday)
   Session 4: 2025-10-27 (Monday)
   ```

2. **Create date mapping before reschedule**
   ```
   "2025-10-06" -> "2025-10-06"
   "2025-10-13" -> "2025-10-13"
   "2025-10-20" -> "2025-10-20"
   "2025-10-27" -> "2025-10-27"
   ```

3. **Apply RescheduleSessions()**
   - Non-holiday sessions stay in place
   - Holiday sessions move to end
   - All sessions get reindexed

4. **Update mapping with new dates**
   ```
   "2025-10-06" -> "2025-10-06" (Session 1 stays)
   "2025-10-13" -> "2025-11-03" (Session 2 moves to end, becomes Session 4)
   "2025-10-20" -> "2025-10-20" (Session 3 becomes Session 2)
   "2025-10-27" -> "2025-10-27" (Session 4 becomes Session 3)
   ```

5. **Generate holiday impacts**
   ```json
   {
     "session_number": 2,
     "date": "2025-10-13",
     "holiday_name": "วันหยุดชดเชยวันปิยมหาราช",
     "shifted_to": "2025-11-03",
     "was_rescheduled": true
   }
   ```

## Result

### Final Session Order:
```
Session 1: 2025-10-06 (Monday) - Original Session 1
Session 2: 2025-10-20 (Monday) - Original Session 3, renumbered
Session 3: 2025-10-27 (Monday) - Original Session 4, renumbered
Session 4: 2025-11-03 (Monday) - Original Session 2, moved and renumbered
```

### Key Features:
✅ Session numbers are sequential (1, 2, 3, 4) with no gaps  
✅ Holiday impacts correctly show which original date moved where  
✅ The `shifted_to` field shows the actual new date  
✅ Works for both `CreateSchedule` and `PreviewSchedule`  

## Testing

### Build and Test
```bash
go build -o englishkorat_go.exe main.go
go test ./controllers -v
```

### Expected Response from Preview
```json
{
  "sessions": [
    {
      "session_number": 1,
      "date": "2025-10-06",
      "week_number": 1
    },
    {
      "session_number": 2,
      "date": "2025-10-20",
      "week_number": 3,
      "notes": ""
    },
    {
      "session_number": 3,
      "date": "2025-10-27",
      "week_number": 4
    },
    {
      "session_number": 4,
      "date": "2025-11-03",
      "week_number": 5,
      "notes": "Rescheduled due to holiday"
    }
  ],
  "original_sessions": [
    {
      "session_number": 1,
      "date": "2025-10-06"
    },
    {
      "session_number": 2,
      "date": "2025-10-13"
    },
    {
      "session_number": 3,
      "date": "2025-10-20"
    },
    {
      "session_number": 4,
      "date": "2025-10-27"
    }
  ],
  "holiday_impacts": [
    {
      "session_number": 2,
      "date": "2025-10-13",
      "holiday_name": "วันหยุดชดเชยวันปิยมหาราช",
      "shifted_to": "2025-11-03",
      "was_rescheduled": true
    }
  ]
}
```

## Files Modified
1. `services/schedule_service.go` - Added session number reindexing in `RescheduleSessions()`
2. `controllers/schedules.go` - Changed preview mapping from session_number to date-based mapping
3. `HOLIDAY_RESCHEDULE_UPDATE.md` - Updated documentation

## Benefits
- Consistent session numbering across all endpoints
- Accurate tracking of date changes in holiday impacts
- Clear communication to users about which sessions moved
- No gaps or duplicate session numbers
