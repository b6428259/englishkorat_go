package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"
	notifsvc "englishkorat_go/services/notifications"
)

// HolidayResponse represents the Thai holiday API response
type HolidayResponse struct {
	VCALENDAR []struct {
		VEVENT []struct {
			DTStart      string `json:"DTSTART"`
			DTStartValue string `json:"DTSTART;VALUE=DATE,omitempty"`
			Summary      string `json:"SUMMARY"`
		} `json:"VEVENT"`
	} `json:"VCALENDAR"`
}

// HolidayInfo contains holiday date and name
type HolidayInfo struct {
	Date time.Time
	Name string
}

// SessionSlot defines a weekday/time slot for generated sessions
type SessionSlot struct {
	Weekday     time.Weekday
	StartHour   int
	StartMinute int
}

// BranchHours represents operating hours for a branch in minutes from midnight
type BranchHours struct {
	OpenMinutes  int
	CloseMinutes int
}

var myHoraHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

func fetchMyHoraJSONWithNames(year int) (map[string]string, error) {
	buddhistYear := year + 543
	url := fmt.Sprintf("https://www.myhora.com/calendar/ical/holiday.aspx?%d.json", buddhistYear)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create holiday request for year %d: %w", year, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EnglishKorat/1.0; +https://englishkorat.com)")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", fmt.Sprintf("https://www.myhora.com/calendar/%d.aspx", buddhistYear))

	resp, err := myHoraHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch holidays for year %d: %w", year, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read holiday response for year %d: %w", year, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch holidays for year %d: status %d", year, resp.StatusCode)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "<") {
		return nil, fmt.Errorf("failed to decode holiday response for year %d: unexpected html content", year)
	}

	var holidayResp HolidayResponse
	if err := json.Unmarshal(body, &holidayResp); err != nil {
		return nil, fmt.Errorf("failed to decode holiday response for year %d: %w", year, err)
	}

	return extractHolidaysFromResponse(holidayResp), nil
}

func fetchMyHoraJSON(year int) ([]time.Time, error) {
	buddhistYear := year + 543
	url := fmt.Sprintf("https://www.myhora.com/calendar/ical/holiday.aspx?%d.json", buddhistYear)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create holiday request for year %d: %w", year, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EnglishKorat/1.0; +https://englishkorat.com)")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", fmt.Sprintf("https://www.myhora.com/calendar/%d.aspx", buddhistYear))

	resp, err := myHoraHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch holidays for year %d: %w", year, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read holiday response for year %d: %w", year, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch holidays for year %d: status %d", year, resp.StatusCode)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "<") {
		return nil, fmt.Errorf("failed to decode holiday response for year %d: unexpected html content", year)
	}

	var holidayResp HolidayResponse
	if err := json.Unmarshal(body, &holidayResp); err != nil {
		return nil, fmt.Errorf("failed to decode holiday response for year %d: %w", year, err)
	}

	holidayMap := extractHolidaysFromResponse(holidayResp)
	result := make([]time.Time, 0, len(holidayMap))
	for dateStr := range holidayMap {
		if date, err := time.Parse("2006-01-02", dateStr); err == nil {
			result = append(result, date)
		}
	}
	return result, nil
}

func fetchMyHoraICSWithNames(year int) (map[string]string, error) {
	buddhistYear := year + 543
	url := fmt.Sprintf("https://www.myhora.com/calendar/ical/holiday.aspx?%d.ics", buddhistYear)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create holiday fallback request for year %d: %w", year, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EnglishKorat/1.0; +https://englishkorat.com)")
	req.Header.Set("Accept", "text/calendar, text/plain, */*")
	req.Header.Set("Referer", fmt.Sprintf("https://www.myhora.com/calendar/%d.aspx", buddhistYear))

	resp, err := myHoraHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fallback holidays for year %d: %w", year, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch fallback holidays for year %d: status %d", year, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	holidayMap := make(map[string]string)
	var currentDate string
	var currentSummary string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "DTSTART") {
			colonIdx := strings.Index(line, ":")
			if colonIdx == -1 {
				continue
			}
			value := strings.TrimSpace(line[colonIdx+1:])
			if len(value) > 8 {
				value = value[:8]
			}
			if len(value) == 8 {
				if date, err := time.Parse("20060102", value); err == nil {
					currentDate = date.Format("2006-01-02")
				}
			}
		} else if strings.HasPrefix(line, "SUMMARY:") {
			currentSummary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
		} else if line == "END:VEVENT" {
			if currentDate != "" && currentSummary != "" {
				if _, exists := holidayMap[currentDate]; !exists {
					holidayMap[currentDate] = currentSummary
				}
			}
			currentDate = ""
			currentSummary = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse fallback holidays for year %d: %w", year, err)
	}

	return holidayMap, nil
}

func fetchMyHoraICS(year int) ([]time.Time, error) {
	buddhistYear := year + 543
	url := fmt.Sprintf("https://www.myhora.com/calendar/ical/holiday.aspx?%d.ics", buddhistYear)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create holiday fallback request for year %d: %w", year, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EnglishKorat/1.0; +https://englishkorat.com)")
	req.Header.Set("Accept", "text/calendar, text/plain, */*")
	req.Header.Set("Referer", fmt.Sprintf("https://www.myhora.com/calendar/%d.aspx", buddhistYear))

	resp, err := myHoraHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fallback holidays for year %d: %w", year, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch fallback holidays for year %d: status %d", year, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	seen := make(map[string]struct{})
	holidayDates := make([]time.Time, 0)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "DTSTART") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		value := strings.TrimSpace(line[colonIdx+1:])
		if len(value) > 8 {
			value = value[:8]
		}
		if len(value) != 8 {
			continue
		}
		if date, err := time.Parse("20060102", value); err == nil {
			key := date.Format("2006-01-02")
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				holidayDates = append(holidayDates, date)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse fallback holidays for year %d: %w", year, err)
	}

	return holidayDates, nil
}

func extractHolidaysFromResponse(resp HolidayResponse) map[string]string {
	holidayMap := make(map[string]string)

	for _, calendar := range resp.VCALENDAR {
		for _, event := range calendar.VEVENT {
			dateStr := strings.TrimSpace(event.DTStart)
			if dateStr == "" {
				dateStr = strings.TrimSpace(event.DTStartValue)
			}
			if dateStr == "" {
				continue
			}
			if len(dateStr) > 8 {
				dateStr = dateStr[:8]
			}
			date, err := time.Parse("20060102", dateStr)
			if err != nil {
				continue
			}
			key := date.Format("2006-01-02")
			if _, exists := holidayMap[key]; !exists {
				holidayMap[key] = strings.TrimSpace(event.Summary)
			}
		}
	}

	return holidayMap
}

// CheckRoomConflict ตรวจสอบการชนของห้องสำหรับ schedule ใหม่
func CheckRoomConflict(roomID uint, startDate, endDate time.Time, sessionStartTime string, hoursPerSession int, recurringPattern string, customDays []int) (bool, error) {
	// ดึง schedule ที่มีอยู่แล้วในห้องนี้ และเป็น type class
	var existingSchedules []models.Schedules
	err := database.DB.Where("room_id = ? AND schedule_type = ? AND status IN ?",
		roomID, "class", []string{"scheduled", "assigned"}).
		Find(&existingSchedules).Error
	if err != nil {
		return false, err
	}

	// แปลง session start time
	startTime, err := time.Parse("15:04", sessionStartTime)
	if err != nil {
		return false, fmt.Errorf("invalid session start time format")
	}

	// สร้าง sessions ชั่วคราวสำหรับ schedule ใหม่
	newSessions := generateSessionTimes(startDate, endDate, startTime, hoursPerSession, recurringPattern, customDays)

	// ตรวจสอบการชนกับ sessions ที่มีอยู่
	for _, schedule := range existingSchedules {
		var existingSessions []models.Schedule_Sessions
		err := database.DB.Where("schedule_id = ? AND status NOT IN ?",
			schedule.ID, []string{"cancelled", "no-show"}).
			Find(&existingSessions).Error
		if err != nil {
			return false, err
		}

		// เปรียบเทียบเวลา
		for _, newSession := range newSessions {
			for _, existingSession := range existingSessions {
				if sessionsOverlap(newSession, existingSession) {
					return true, nil // มีการชน
				}
			}
		}
	}

	return false, nil
}

// sessionsOverlap ตรวจสอบว่า session 2 session ทับกันหรือไม่
func sessionsOverlap(session1, session2 models.Schedule_Sessions) bool {
	// ตรวจสอบวันที่เดียวกัน
	if session1.Session_date == nil || session2.Session_date == nil {
		return false
	}
	date1 := session1.Session_date.Format("2006-01-02")
	date2 := session2.Session_date.Format("2006-01-02")
	if date1 != date2 {
		return false
	}

	// ตรวจสอบเวลาทับกัน
	if session1.Start_time == nil || session1.End_time == nil || session2.Start_time == nil || session2.End_time == nil {
		return false
	}
	start1 := session1.Start_time.Format("15:04")
	end1 := session1.End_time.Format("15:04")
	start2 := session2.Start_time.Format("15:04")
	end2 := session2.End_time.Format("15:04")

	return !(end1 <= start2 || end2 <= start1)
}

// generateSessionTimes สร้างเวลา session ชั่วคราวสำหรับตรวจสอบการชน
func generateSessionTimes(startDate, endDate time.Time, sessionStartTime time.Time, hoursPerSession int, recurringPattern string, customDays []int) []models.Schedule_Sessions {
	var sessions []models.Schedule_Sessions
	current := startDate
	sessionNumber := 1

	// Normalize custom days: accept 0..6 with 0=Sun, and map 7->0 (some clients send 7 for Sunday)
	normalizedDays := make(map[int]bool, len(customDays))
	for _, d := range customDays {
		if d == 7 {
			d = 0
		}
		if d < 0 {
			d = 0
		}
		if d > 6 {
			d = d % 7
		}
		normalizedDays[d] = true
	}

	for current.Before(endDate) || current.Equal(endDate) {
		shouldCreateSession := false

		switch recurringPattern {
		case "daily":
			shouldCreateSession = true
		case "weekly":
			shouldCreateSession = current.Weekday() == startDate.Weekday()
		case "bi-weekly":
			weeksDiff := int(current.Sub(startDate).Hours() / (24 * 7))
			shouldCreateSession = weeksDiff%2 == 0 && current.Weekday() == startDate.Weekday()
		case "custom":
			weekday := int(current.Weekday())
			if normalizedDays[weekday] {
				shouldCreateSession = true
			}
		case "none":
			// One-off / non-recurring schedule: create only the first session
			if sessionNumber == 1 {
				shouldCreateSession = true
			}
		}

		if shouldCreateSession {
			sessionStart := time.Date(current.Year(), current.Month(), current.Day(),
				sessionStartTime.Hour(), sessionStartTime.Minute(), 0, 0, current.Location())
			sessionEnd := sessionStart.Add(time.Duration(hoursPerSession) * time.Hour)

			// Convert times to pointers to match nullable model fields
			sd := current
			ss := sessionStart
			se := sessionEnd
			session := models.Schedule_Sessions{
				Session_date:          &sd,
				Start_time:            &ss,
				End_time:              &se,
				Session_number:        sessionNumber,
				Status:                "scheduled",
				Makeup_for_session_id: nil,
			}
			sessions = append(sessions, session)
			sessionNumber++
		}

		current = current.AddDate(0, 0, 1)
	}

	return sessions
}

// GenerateScheduleSessions สร้าง sessions ตาม recurring pattern
func GenerateScheduleSessions(schedule models.Schedules, sessionStartTime string, customDays []int) ([]models.Schedule_Sessions, error) {
	startTime, err := time.Parse("15:04", sessionStartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid session start time format")
	}

	var sessions []models.Schedule_Sessions
	current := schedule.Start_date
	sessionNumber := 1
	weekNumber := 1

	// คำนวณ sessions ที่ต้องการทั้งหมด
	totalSessions := schedule.Total_hours / schedule.Hours_per_session

	// Determine an end guard: if Estimated_end_date is zero, derive a soft cap by days to cover required sessions
	// Assume max of 365 days lookahead to avoid infinite loop when no end date provided.
	hasEndDate := !schedule.Estimated_end_date.IsZero()
	softEnd := schedule.Start_date.AddDate(1, 0, 0) // one year cap

	// Normalize custom days for this generator as well
	normalizedDays := make(map[int]bool, len(customDays))
	for _, d := range customDays {
		if d == 7 {
			d = 0
		}
		if d < 0 {
			d = 0
		}
		if d > 6 {
			d = d % 7
		}
		normalizedDays[d] = true
	}

	// Use Asia/Bangkok timezone for consistent date handling
	bangkokLoc, _ := time.LoadLocation("Asia/Bangkok")

	for sessionNumber <= totalSessions && ((hasEndDate && (current.Before(schedule.Estimated_end_date) || current.Equal(schedule.Estimated_end_date))) || (!hasEndDate && current.Before(softEnd))) {
		shouldCreateSession := false

		switch schedule.Recurring_pattern {
		case "daily":
			shouldCreateSession = true
		case "weekly":
			shouldCreateSession = current.Weekday() == schedule.Start_date.Weekday()
		case "bi-weekly":
			weeksDiff := int(current.Sub(schedule.Start_date).Hours() / (24 * 7))
			shouldCreateSession = weeksDiff%2 == 0 && current.Weekday() == schedule.Start_date.Weekday()
		case "monthly":
			shouldCreateSession = current.Day() == schedule.Start_date.Day()
		case "custom":
			weekday := int(current.Weekday())
			if normalizedDays[weekday] {
				shouldCreateSession = true
			}
		case "none":
			// One-off / non-recurring schedule: create only the first session
			if sessionNumber == 1 {
				shouldCreateSession = true
			}
		}

		if shouldCreateSession {
			// Use the specific date for session, maintaining consistent timezone
			sessionDate := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, bangkokLoc)
			sessionStart := time.Date(current.Year(), current.Month(), current.Day(),
				startTime.Hour(), startTime.Minute(), 0, 0, bangkokLoc)
			sessionEnd := sessionStart.Add(time.Duration(schedule.Hours_per_session) * time.Hour)

			session := models.Schedule_Sessions{
				Session_date:          &sessionDate,
				Start_time:            &sessionStart,
				End_time:              &sessionEnd,
				Session_number:        sessionNumber,
				Week_number:           weekNumber,
				Status:                "scheduled",
				Makeup_for_session_id: nil,
			}
			sessions = append(sessions, session)
			sessionNumber++

			// เพิ่ม week number ตาม session per week
			if sessionNumber%schedule.Session_per_week == 1 && sessionNumber > 1 {
				weekNumber++
			}
		}

		current = current.AddDate(0, 0, 1)
	}

	return sessions, nil
}

// GenerateScheduleSessionsWithSlots generates sessions based on explicit weekday/time slots.
// It validates branch operating hours before creating sessions.
func GenerateScheduleSessionsWithSlots(schedule models.Schedules, slots []SessionSlot, totalSessions, hoursPerSession int, hours BranchHours) ([]models.Schedule_Sessions, error) {
	if len(slots) == 0 {
		return nil, fmt.Errorf("session slots are required to generate schedule sessions")
	}
	if totalSessions <= 0 {
		return nil, fmt.Errorf("total sessions must be greater than zero")
	}
	if hoursPerSession <= 0 {
		return nil, fmt.Errorf("hours per session must be greater than zero")
	}
	if hours.CloseMinutes <= hours.OpenMinutes {
		return nil, fmt.Errorf("branch operating hours are invalid")
	}

	// Validate each slot against branch hours
	for _, slot := range slots {
		if slot.Weekday < time.Sunday || slot.Weekday > time.Saturday {
			return nil, fmt.Errorf("invalid weekday provided for session slot")
		}
		startMinutes := slot.StartHour*60 + slot.StartMinute
		endMinutes := startMinutes + hoursPerSession*60
		if startMinutes < hours.OpenMinutes || endMinutes > hours.CloseMinutes {
			return nil, fmt.Errorf("session starting at %02d:%02d on %s is outside branch hours", slot.StartHour, slot.StartMinute, slot.Weekday.String())
		}
	}

	// Group slots by weekday and sort by start time
	slotsByDay := make(map[time.Weekday][]SessionSlot)
	for _, slot := range slots {
		slotsByDay[slot.Weekday] = append(slotsByDay[slot.Weekday], slot)
	}
	for day := range slotsByDay {
		sort.SliceStable(slotsByDay[day], func(i, j int) bool {
			a := slotsByDay[day][i]
			b := slotsByDay[day][j]
			if a.StartHour == b.StartHour {
				return a.StartMinute < b.StartMinute
			}
			return a.StartHour < b.StartHour
		})
	}

	bangkokLoc, _ := time.LoadLocation("Asia/Bangkok")
	startInLoc := schedule.Start_date.In(bangkokLoc)
	startDate := time.Date(startInLoc.Year(), startInLoc.Month(), startInLoc.Day(), 0, 0, 0, 0, bangkokLoc)

	sessions := make([]models.Schedule_Sessions, 0, totalSessions)
	current := startDate
	maxDays := 366 * 2 // guard against runaway loops (2 years)
	processedDays := 0

	for len(sessions) < totalSessions && processedDays <= maxDays {
		daySlots := slotsByDay[current.Weekday()]
		if len(daySlots) > 0 {
			for _, slot := range daySlots {
				if len(sessions) >= totalSessions {
					break
				}

				sessionStart := time.Date(current.Year(), current.Month(), current.Day(), slot.StartHour, slot.StartMinute, 0, 0, bangkokLoc)
				sessionEnd := sessionStart.Add(time.Duration(hoursPerSession) * time.Hour)
				sessionDate := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, bangkokLoc)

				// Additional safety: ensure session end still within same day bounds
				if sessionEnd.Sub(sessionStart) != time.Duration(hoursPerSession)*time.Hour {
					return nil, fmt.Errorf("failed to compute session duration for %s", sessionStart.Format(time.RFC3339))
				}

				sd := sessionDate
				st := sessionStart
				et := sessionEnd

				sessionNumber := len(sessions) + 1
				weeksFromStart := int(sd.Sub(startDate).Hours() / (24 * 7))
				if weeksFromStart < 0 {
					weeksFromStart = 0
				}

				sessions = append(sessions, models.Schedule_Sessions{
					Session_date:   &sd,
					Start_time:     &st,
					End_time:       &et,
					Session_number: sessionNumber,
					Week_number:    weeksFromStart + 1,
					Status:         "scheduled",
				})
			}
		}

		current = current.AddDate(0, 0, 1)
		processedDays++
	}

	if len(sessions) != totalSessions {
		return nil, fmt.Errorf("unable to generate the requested number of sessions (%d/%d) within allowed timeframe", len(sessions), totalSessions)
	}

	return sessions, nil
}

// ReindexSessions sorts sessions chronologically and recalculates session and week numbers relative to startDate.
func ReindexSessions(sessions []models.Schedule_Sessions, startDate time.Time) {
	if len(sessions) == 0 {
		return
	}

	bangkokLoc, _ := time.LoadLocation("Asia/Bangkok")
	startInLoc := startDate.In(bangkokLoc)
	normalizedStart := time.Date(startInLoc.Year(), startInLoc.Month(), startInLoc.Day(), 0, 0, 0, 0, bangkokLoc)

	sort.SliceStable(sessions, func(i, j int) bool {
		var leftDate, rightDate time.Time
		if sessions[i].Session_date != nil {
			leftDate = sessions[i].Session_date.In(bangkokLoc)
		}
		if sessions[j].Session_date != nil {
			rightDate = sessions[j].Session_date.In(bangkokLoc)
		}

		if !leftDate.Equal(rightDate) {
			return leftDate.Before(rightDate)
		}

		var leftStart, rightStart time.Time
		if sessions[i].Start_time != nil {
			leftStart = sessions[i].Start_time.In(bangkokLoc)
		}
		if sessions[j].Start_time != nil {
			rightStart = sessions[j].Start_time.In(bangkokLoc)
		}

		return leftStart.Before(rightStart)
	})

	for idx := range sessions {
		sessions[idx].Session_number = idx + 1
		if sessions[idx].Session_date != nil {
			sessionDate := sessions[idx].Session_date.In(bangkokLoc)
			normalized := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 0, 0, 0, 0, bangkokLoc)
			weeks := int(normalized.Sub(normalizedStart).Hours() / (24 * 7))
			if weeks < 0 {
				weeks = 0
			}
			sessions[idx].Week_number = weeks + 1
		} else {
			sessions[idx].Week_number = 1
		}
	}
}

// GetThaiHolidaysWithNames ดึงวันหยุดไทยพร้อมชื่อจาก API
func GetThaiHolidaysWithNames(startYear, endYear int) (map[string]string, error) {
	holidayNames := make(map[string]string)
	var errorSummaries []string

	for year := startYear; year <= endYear; year++ {
		var yearHolidays map[string]string
		var yearErrors []string

		if holidays, err := fetchMyHoraJSONWithNames(year); err != nil {
			yearErrors = append(yearErrors, err.Error())
		} else {
			yearHolidays = holidays
		}

		if len(yearHolidays) == 0 {
			if holidays, err := fetchMyHoraICSWithNames(year); err != nil {
				yearErrors = append(yearErrors, err.Error())
			} else {
				yearHolidays = holidays
			}
		}

		if len(yearHolidays) == 0 {
			if len(yearErrors) > 0 {
				errorSummaries = append(errorSummaries, fmt.Sprintf("%d: %s", year, strings.Join(yearErrors, " | ")))
			}
			continue
		}

		for dateStr, name := range yearHolidays {
			if _, exists := holidayNames[dateStr]; !exists {
				holidayNames[dateStr] = name
			}
		}
	}

	if len(errorSummaries) > 0 {
		if len(holidayNames) == 0 {
			return nil, fmt.Errorf("failed to fetch holidays: %s", strings.Join(errorSummaries, "; "))
		}
		log.Printf("warning: partial holiday fetch failures: %s", strings.Join(errorSummaries, "; "))
	}

	return holidayNames, nil
}

// GetThaiHolidays ดึงวันหยุดไทยจาก API
func GetThaiHolidays(startYear, endYear int) ([]time.Time, error) {
	seen := make(map[string]struct{})
	var allHolidays []time.Time
	var errorSummaries []string

	for year := startYear; year <= endYear; year++ {
		var yearHolidays []time.Time
		var yearErrors []string

		if holidays, err := fetchMyHoraJSON(year); err != nil {
			yearErrors = append(yearErrors, err.Error())
		} else {
			yearHolidays = append(yearHolidays, holidays...)
		}

		if len(yearHolidays) == 0 {
			if holidays, err := fetchMyHoraICS(year); err != nil {
				yearErrors = append(yearErrors, err.Error())
			} else {
				yearHolidays = append(yearHolidays, holidays...)
			}
		}

		if len(yearHolidays) == 0 {
			if len(yearErrors) > 0 {
				errorSummaries = append(errorSummaries, fmt.Sprintf("%d: %s", year, strings.Join(yearErrors, " | ")))
			}
			continue
		}

		for _, date := range yearHolidays {
			key := date.Format("2006-01-02")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			allHolidays = append(allHolidays, date)
		}
	}

	sort.Slice(allHolidays, func(i, j int) bool {
		return allHolidays[i].Before(allHolidays[j])
	})

	if len(errorSummaries) > 0 {
		if len(allHolidays) == 0 {
			return nil, fmt.Errorf("failed to fetch holidays: %s", strings.Join(errorSummaries, "; "))
		}
		log.Printf("warning: partial holiday fetch failures: %s", strings.Join(errorSummaries, "; "))
	}

	return allHolidays, nil
}

// RescheduleSessions ปรับ sessions ที่ตรงกับวันหยุด โดยยกเลิกและไปต่อท้าย
func RescheduleSessions(sessions []models.Schedule_Sessions, holidays []time.Time) []models.Schedule_Sessions {
	if len(sessions) == 0 {
		return sessions
	}

	holidayMap := make(map[string]bool)
	for _, holiday := range holidays {
		holidayMap[holiday.Format("2006-01-02")] = true
	}

	// สกัด pattern ของวันที่ใช้ในตาราง (weekdays ที่ใช้)
	weekdayPattern := make(map[time.Weekday]bool)
	timeByWeekday := make(map[time.Weekday][]time.Time) // เก็บเวลาเริ่ม-สิ้นสุดตาม weekday

	for _, session := range sessions {
		if session.Session_date != nil {
			weekday := session.Session_date.Weekday()
			weekdayPattern[weekday] = true

			// เก็บเวลาของแต่ละ weekday (กรณีมีหลายช่วงเวลาในวันเดียวกัน)
			if session.Start_time != nil && session.End_time != nil {
				timeByWeekday[weekday] = append(timeByWeekday[weekday], *session.Start_time, *session.End_time)
			}
		}
	}

	// แยก sessions ที่ตรงวันหยุดออกมา
	var rescheduledSessions []models.Schedule_Sessions
	var postponedSessions []models.Schedule_Sessions

	for _, session := range sessions {
		sessionDate := session.Session_date.Format("2006-01-02")
		if holidayMap[sessionDate] {
			postponedSessions = append(postponedSessions, session)
		} else {
			rescheduledSessions = append(rescheduledSessions, session)
		}
	}

	// ถ้าไม่มี sessions ที่ต้องเลื่อน ก็ return เลย
	if len(postponedSessions) == 0 {
		return rescheduledSessions
	}

	// หาวันสุดท้ายที่มี session
	if len(rescheduledSessions) == 0 {
		// ถ้าทุก session เป็นวันหยุดหมด ให้เริ่มจากวันแรกของ schedule
		rescheduledSessions = postponedSessions
		postponedSessions = nil
	}

	if len(postponedSessions) > 0 {
		lastSession := rescheduledSessions[len(rescheduledSessions)-1]
		currentDate := *lastSession.Session_date
		bangkokLoc := currentDate.Location()

		// เรียง weekdays ตาม pattern ที่ใช้
		var sortedWeekdays []time.Weekday
		for wd := time.Sunday; wd <= time.Saturday; wd++ {
			if weekdayPattern[wd] {
				sortedWeekdays = append(sortedWeekdays, wd)
			}
		}

		// สำหรับแต่ละ postponed session หาวันถัดไปที่ตรง pattern และไม่ใช่วันหยุด
		for _, session := range postponedSessions {
			originalWeekday := session.Session_date.Weekday()

			// หาวัน weekday ถัดไปที่ตรงกับ originalWeekday และไม่ตรงกับวันหยุด
			found := false
			searchDate := currentDate.AddDate(0, 0, 1)
			maxSearch := 60 // ป้องกัน infinite loop

			for attempts := 0; attempts < maxSearch && !found; attempts++ {
				if searchDate.Weekday() == originalWeekday && !holidayMap[searchDate.Format("2006-01-02")] {
					found = true

					// อัพเดทวันที่ session
					newDate := searchDate
					session.Session_date = &newDate

					newStart := time.Date(newDate.Year(), newDate.Month(), newDate.Day(),
						session.Start_time.Hour(), session.Start_time.Minute(), 0, 0, bangkokLoc)
					newEnd := time.Date(newDate.Year(), newDate.Month(), newDate.Day(),
						session.End_time.Hour(), session.End_time.Minute(), 0, 0, bangkokLoc)

					session.Start_time = &newStart
					session.End_time = &newEnd
					session.Notes = "Rescheduled due to holiday"

					rescheduledSessions = append(rescheduledSessions, session)
					currentDate = newDate
				} else {
					searchDate = searchDate.AddDate(0, 0, 1)
				}
			}

			if !found {
				// Fallback: ถ้าหาไม่เจอ ให้ใช้วันถัดไปที่ไม่ใช่วันหยุด
				searchDate = currentDate.AddDate(0, 0, 1)
				for holidayMap[searchDate.Format("2006-01-02")] {
					searchDate = searchDate.AddDate(0, 0, 1)
				}

				newDate := searchDate
				session.Session_date = &newDate

				newStart := time.Date(newDate.Year(), newDate.Month(), newDate.Day(),
					session.Start_time.Hour(), session.Start_time.Minute(), 0, 0, bangkokLoc)
				newEnd := time.Date(newDate.Year(), newDate.Month(), newDate.Day(),
					session.End_time.Hour(), session.End_time.Minute(), 0, 0, bangkokLoc)

				session.Start_time = &newStart
				session.End_time = &newEnd
				session.Notes = "Rescheduled due to holiday"

				rescheduledSessions = append(rescheduledSessions, session)
				currentDate = newDate
			}
		}
	}

	// Reindex session numbers เรียงลำดับใหม่ 1, 2, 3...
	for i := range rescheduledSessions {
		rescheduledSessions[i].Session_number = i + 1
	}

	return rescheduledSessions
}

// NotifyStudentsScheduleConfirmed ส่ง notification ให้นักเรียนเมื่อ schedule ถูกยืนยัน
func NotifyStudentsScheduleConfirmed(scheduleID uint) {
	var schedule models.Schedules
	if err := database.DB.Preload("Group.Course").First(&schedule, scheduleID).Error; err != nil {
		return
	}

	// ดึงรายชื่อนักเรียนจาก group members (สำหรับ class schedules)
	if schedule.GroupID != nil {
		var groupMembers []models.GroupMember
		if err := database.DB.Preload("Student.User").Where("group_id = ?", *schedule.GroupID).Find(&groupMembers).Error; err != nil {
			return
		}

		// สร้าง notification สำหรับนักเรียนแต่ละคน
		for _, member := range groupMembers {
			if member.Student.UserID != nil {
				notification := models.Notification{
					UserID:    *member.Student.UserID,
					Title:     "Schedule Confirmed",
					TitleTh:   "ตารางเรียนได้รับการยืนยันแล้ว",
					Message:   fmt.Sprintf("Your class schedule '%s' has been confirmed by the teacher.", schedule.ScheduleName),
					MessageTh: fmt.Sprintf("ตารางเรียน '%s' ของคุณได้รับการยืนยันจากครูแล้ว", schedule.ScheduleName),
					Type:      "success",
				}

				database.DB.Create(&notification)
			}
		}
	} else {
		// สำหรับ event/appointment schedules - ส่งให้ participants
		var participants []models.ScheduleParticipant
		if err := database.DB.Where("schedule_id = ?", scheduleID).Find(&participants).Error; err != nil {
			return
		}

		for _, participant := range participants {
			notification := models.Notification{
				UserID:    participant.UserID,
				Title:     "Schedule Confirmed",
				TitleTh:   "ตารางนัดหมายได้รับการยืนยันแล้ว",
				Message:   fmt.Sprintf("Your scheduled '%s' has been confirmed.", schedule.ScheduleName),
				MessageTh: fmt.Sprintf("การนัดหมาย '%s' ของคุณได้รับการยืนยันแล้ว", schedule.ScheduleName),
				Type:      "success",
			}

			database.DB.Create(&notification)
		}
	}
}

// NotifyUpcomingClass ส่ง notification เตือนก่อนเรียน
func NotifyUpcomingClass(sessionID uint, minutesBefore int) {
	var session models.Schedule_Sessions
	if err := database.DB.First(&session, sessionID).Error; err != nil {
		return
	}

	var schedule models.Schedules
	if err := database.DB.Preload("Group.Course").First(&schedule, session.ScheduleID).Error; err != nil {
		return
	}

	// ตรวจสอบว่า session ได้รับการยืนยันแล้ว (ในระบบนี้ถือว่า 'scheduled' คือยืนยันแล้ว)
	if session.Status != "scheduled" {
		return
	}

	// ดึงรายชื่อผู้เข้าร่วม (ครูและนักเรียน)
	var users []models.User

	// For class schedules - get users from group members
	if schedule.GroupID != nil {
		var groupMembers []models.GroupMember
		err := database.DB.Preload("Student.User").Where("group_id = ?", *schedule.GroupID).Find(&groupMembers).Error
		if err == nil {
			for _, member := range groupMembers {
				if member.Student.UserID != nil {
					var user models.User
					if err := database.DB.First(&user, *member.Student.UserID).Error; err == nil {
						users = append(users, user)
					}
				}
			}
		}
	} else {
		// For event/appointment schedules - get participants
		var participants []models.ScheduleParticipant
		err := database.DB.Preload("User").Where("schedule_id = ?", schedule.ID).Find(&participants).Error
		if err == nil {
			for _, participant := range participants {
				users = append(users, participant.User)
			}
		}
	}

	// เพิ่มครูที่ถูก assign (ใช้ default teacher หรือ teacher specific สำหรับ session)
	teacherID := session.AssignedTeacherID
	if teacherID == nil {
		teacherID = schedule.DefaultTeacherID
	}

	if teacherID != nil {
		var assignedTeacher models.User
		if err := database.DB.First(&assignedTeacher, *teacherID).Error; err == nil {
			// สร้าง notification สำหรับครู
			notification := models.Notification{
				UserID:    assignedTeacher.ID,
				Title:     "Upcoming Class",
				TitleTh:   "เรียนจะเริ่มเร็วๆ นี้",
				Message:   fmt.Sprintf("Your class '%s' will start in %d minutes at %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				MessageTh: fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %d นาที เวลา %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				Type:      "info",
			}
			database.DB.Create(&notification)
		}
	}

	// สร้าง notification สำหรับนักเรียน
	for _, user := range users {
		if user.Role == "student" {
			notification := models.Notification{
				UserID:    user.ID,
				Title:     "Upcoming Class",
				TitleTh:   "เรียนจะเริ่มเร็วๆ นี้",
				Message:   fmt.Sprintf("Your class '%s' will start in %d minutes at %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				MessageTh: fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %d นาที เวลา %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				Type:      "info",
			}
			database.DB.Create(&notification)
		}
	}
}

// ScheduleNotifications ตั้งเวลา notification สำหรับ sessions ที่จะมาถึง
func ScheduleNotifications() {
	// ดึง sessions ที่จะเกิดขึ้นในอนาคต
	var sessions []models.Schedule_Sessions
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)

	err := database.DB.Where("session_date BETWEEN ? AND ? AND status = ?",
		now.Format("2006-01-02"), tomorrow.Format("2006-01-02"), "scheduled").
		Find(&sessions).Error
	if err != nil {
		return
	}

	for _, session := range sessions {
		// คำนวณเวลาก่อนเรียน (เช่น 30 นาที, 1 ชั่วโมง)
		notificationTimes := []int{30, 60} // นาที

		for _, minutes := range notificationTimes {
			notifyTime := session.Start_time.Add(-time.Duration(minutes) * time.Minute)

			if notifyTime.After(now) {
				// ตั้งเวลาส่ง notification (ในระบบจริงอาจใช้ cron job หรือ message queue)
				go func(sessionID uint, mins int, notifyAt time.Time) {
					time.Sleep(notifyAt.Sub(now))
					NotifyUpcomingClass(sessionID, mins)
				}(session.ID, minutes, notifyTime)
			}
		}
	}
}

// ScheduleTeacherConfirmReminders schedules reminder notifications for a class session
// to prompt the assigned/default teacher to confirm at T-24h and T-6h before start.
func ScheduleTeacherConfirmReminders(session models.Schedule_Sessions, schedule models.Schedules) {
	// Only for class schedules and sessions in 'assigned' status
	if schedule.ScheduleType != "class" {
		return
	}
	if session.Status != "assigned" {
		return
	}
	if session.Start_time == nil || session.Session_date == nil {
		return
	}

	// Determine teacher to notify (session-level or schedule default)
	teacherID := session.AssignedTeacherID
	if teacherID == nil {
		teacherID = schedule.DefaultTeacherID
	}
	if teacherID == nil {
		return
	}

	// Calculate absolute start datetime in Asia/Bangkok
	// Session.Start_time already has correct date/time; use it directly
	startAt := *session.Start_time
	now := time.Now()

	// Reminder offsets (hours before)
	offsets := []time.Duration{24 * time.Hour, 6 * time.Hour}

	notifService := notifsvc.NewService()
	for _, off := range offsets {
		notifyAt := startAt.Add(-off)
		if notifyAt.After(now) {
			// schedule a goroutine sleep; in production, prefer a durable scheduler/queue
			go func(teacher uint, sess models.Schedule_Sessions, sch models.Schedules, when time.Time) {
				time.Sleep(time.Until(when))
				// double-check status still 'assigned' before sending
				var latest models.Schedule_Sessions
				if err := database.DB.First(&latest, sess.ID).Error; err == nil && latest.Status == "assigned" {
					// Build notification
					data := map[string]any{
						"link":        map[string]any{"href": fmt.Sprintf("/api/schedules/sessions/%d", sess.ID), "method": "GET"},
						"action":      "confirm-session",
						"session_id":  sess.ID,
						"schedule_id": sch.ID,
					}
					payload := notifsvc.QueuedWithData(
						"Please confirm your session",
						"กรุณายืนยันคาบเรียนของคุณ",
						fmt.Sprintf("Please confirm the session for '%s' at %s.", sch.ScheduleName, startAt.Format("2006-01-02 15:04")),
						fmt.Sprintf("กรุณายืนยันคาบเรียนสำหรับ '%s' เวลา %s", sch.ScheduleName, startAt.Format("2006-01-02 15:04")),
						"info", data,
						"popup", "normal",
					)
					_ = notifService.EnqueueOrCreate([]uint{teacher}, payload)
				}
			}(*teacherID, session, schedule, notifyAt)
		}
	}
}
