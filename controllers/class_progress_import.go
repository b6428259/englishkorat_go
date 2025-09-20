package controllers

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ClassProgressImportController handles importing class progress from CSV/XLSX
type ClassProgressImportController struct{}

// POST /api/import/class-progress
// Multipart form with file field: file
func (ic *ClassProgressImportController) Import(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	// Open file
	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot open file"})
	}
	defer file.Close()

	filename := strings.ToLower(fileHeader.Filename)
	var rows [][]string
	var parseErr error

	if strings.HasSuffix(filename, ".csv") {
		rows, parseErr = readCSV(file)
	} else if strings.HasSuffix(filename, ".xlsx") || strings.HasSuffix(filename, ".xls") {
		// Save to OS temp folder for excelize to open
		tmpDir, _ := os.MkdirTemp("", "ekxls-")
		tmp := filepath.Join(tmpDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(fileHeader.Filename)))
		if err := c.SaveFile(fileHeader, tmp); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to buffer upload"})
		}
		rows, parseErr = readXLSX(tmp)
		// Best-effort cleanup
		_ = os.Remove(tmp)
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported file type (csv, xlsx)"})
	}
	if parseErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": parseErr.Error()})
	}

	if len(rows) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is empty"})
	}

	// Expect header with specific columns
	header := rows[0]
	col := buildColumnIndex(header)
	// Minimal required columns
	required := []string{"Student1", "StudentEN1", "Level", "TargetHours", "TotalHour", "Branch", "Date", "Hour"}
	for _, r := range required {
		if _, ok := col[r]; !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("missing column: %s", r)})
		}
	}

	// Process rows (skip header)
	created := 0
	skipped := 0
	var errorsList []string
	totalRows := len(rows) // including header
	dataRows := 0          // excluding header
	if len(rows) > 0 {
		dataRows = len(rows) - 1
	}

	// Additional counters for insights
	uniqueNameKeys := map[string]struct{}{}
	studentsCreated := 0
	studentsReused := 0
	membersAdded := 0
	duplicateRows := 0

	// Default password
	defaultPassword := "1424123"
	hashedDefault, _ := utils.HashPassword(defaultPassword)

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for i := 1; i < len(rows); i++ {
			r := rows[i]
			// Map fields safely
			get := func(key string) string {
				if idx, ok := col[key]; ok && idx < len(r) {
					return strings.TrimSpace(r[idx])
				}
				return ""
			}

			// Student names (Thai/EN). Create or get Students and Users. Pair/group up to 4.
			studentThs := []string{get("Student1"), get("Student2"), get("Student3"), get("Student4")}
			studentEns := []string{get("StudentEN1"), get("StudentEN2"), get("StudentEN3"), get("StudentEN4")}

			// Track unique names across file
			for k := 0; k < 4; k++ {
				th := strings.ToLower(strings.TrimSpace(studentThs[k]))
				en := strings.ToLower(strings.TrimSpace(studentEns[k]))
				if th == "" && en == "" {
					continue
				}
				key := th + "|" + en
				uniqueNameKeys[key] = struct{}{}
			}

			// Level, Course Path, Target/Special/Total
			level := get("Level")
			coursePath := get("CoursePath")
			targetHours := parseIntPtr(get("TargetHours"))
			specialHours := parseIntPtr(firstNonEmpty(get("SpeacialHours"), get("SpecialHour")))
			totalHour := parseIntPtr(firstNonEmpty(get("TotalHour"), get("TotalHours")))
			branchRaw := get("Branch") // e.g., "1,3"; sometimes empty

			// Ensure branch: map provided numbers to known branches by code/name. If Branch is empty, also try FileName.
			branchID := resolveBranchByNumbers(tx, branchRaw, get("FileName"))

			// Find or create Course by name (CoursePath). If blank, use Level.
			courseName := strings.TrimSpace(coursePath)
			if courseName == "" {
				courseName = strings.TrimSpace(level)
			}
			var course *models.Course
			if courseName != "" {
				cRec, cerr := findOrCreateCourse(tx, courseName, branchID, level)
				if cerr != nil {
					return cerr
				}
				course = cRec
			}

			// Find or create Group by convention: FileName/Level or Student list. We'll use FileName if present + Level
			fileName := get("FileName")
			groupName := buildGroupName(fileName, level, studentThs, studentEns)
			var group *models.Group
			if groupName != "" {
				var gRec models.Group
				if err := tx.Where("group_name = ?", groupName).First(&gRec).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						// Need course id; if nil, create a placeholder course
						var courseID uint
						if course != nil {
							courseID = course.ID
						} else {
							// Generate a unique non-empty code for the placeholder course to satisfy unique constraint
							placeholderCode := fmt.Sprintf("auto-%d", time.Now().UnixNano())
							pc := models.Course{Name: fmt.Sprintf("Course for %s", groupName), Code: placeholderCode, BranchID: branchID, Level: level, Status: "active"}
							if err := tx.Create(&pc).Error; err != nil {
								return err
							}
							courseID = pc.ID
						}
						gRec = models.Group{GroupName: groupName, CourseID: courseID, Level: level, Status: "active"}
						if err := tx.Create(&gRec).Error; err != nil {
							return err
						}
					} else {
						return err
					}
				}
				group = &gRec
			}

			// Ensure Users/Students exist and are in the group (dedupe by nickname TH/EN or username)
			studentsEnsured := 0
			for k := 0; k < 4; k++ {
				th := strings.TrimSpace(studentThs[k])
				en := strings.TrimSpace(studentEns[k])
				if th == "" && en == "" {
					continue
				}

				// Build username: use Thai nickname if provided else English
				username := firstNonEmpty(th, en)
				if username == "" {
					username = fmt.Sprintf("student_%d_%d", time.Now().Unix(), i)
				}

				// 1) Try find existing student by Thai/English nicknames
				existingStudent := findStudentByNicknames(tx, th, en)
				var user models.User
				var student models.Student

				if existingStudent != nil {
					// Reuse existing student
					student = *existingStudent
					studentsReused++
					// If student already linked to user, reuse that user; else find/create user and link
					if student.UserID != nil {
						if err := tx.First(&user, *student.UserID).Error; err != nil {
							return err
						}
					} else {
						// Find or create user by username and attach
						u, err := findOrCreateUserByUsername(tx, username, branchID, hashedDefault)
						if err != nil {
							return err
						}
						user = u
						if err := tx.Model(&student).Update("user_id", user.ID).Error; err != nil {
							return err
						}
					}
				} else {
					// 2) No existing student by nickname; find or create user by username
					u, err := findOrCreateUserByUsername(tx, username, branchID, hashedDefault)
					if err != nil {
						return err
					}
					user = u

					// 3) Try find student by this user
					if err := tx.Where("user_id = ?", user.ID).First(&student).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							// Create new student with required fields
							fn := username
							ln := username
							nTh := th
							nEn := en
							if strings.TrimSpace(nTh) == "" {
								nTh = username
							}
							if strings.TrimSpace(nEn) == "" {
								nEn = username
							}
							if strings.TrimSpace(fn) == "" {
								fn = username
							}
							if strings.TrimSpace(ln) == "" {
								ln = fn
							}

							student = models.Student{UserID: &user.ID, FirstName: fn, LastName: ln, NicknameTh: nTh, NicknameEn: nEn, AgeGroup: "adults"}
							if level != "" {
								student.LanguageLevel = level
							}
							if err := tx.Create(&student).Error; err != nil {
								return err
							}
							studentsCreated++
						} else {
							return err
						}
					}
				}

				// Add to group as member
				if group != nil {
					var gm models.GroupMember
					if err := tx.Where("group_id = ? AND student_id = ?", group.ID, student.ID).First(&gm).Error; err != nil {
						if err == gorm.ErrRecordNotFound {
							gm = models.GroupMember{GroupID: group.ID, StudentID: student.ID, Status: "active"}
							if err := tx.Create(&gm).Error; err != nil {
								return err
							}
							membersAdded++
						} else {
							return err
						}
					}
				}
				studentsEnsured++
			}

			if studentsEnsured == 0 {
				skipped++
				continue
			}

			// Teacher mapping (fuzzy match by nickname en/th)
			teacherName := get("Teacher")
			var teacherID *uint
			if teacherName != "" {
				if t := findTeacherClosest(tx, teacherName); t != nil {
					teacherID = &t.ID
				}
			}

			// Book mapping
			bookRaw := get("Book")
			var bookID *uint
			if bookRaw != "" {
				var b models.Book
				if err := tx.Where("name = ?", bookRaw).First(&b).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						b = models.Book{Name: bookRaw}
						if err := tx.Create(&b).Error; err != nil {
							return err
						}
					} else {
						return err
					}
				}
				bookID = &b.ID
			}

			// Parse date (handle formats like 18/05/22 or 26/09/24)
			dateStr := get("Date")
			var datePtr *time.Time
			if dateStr != "" {
				if dt, derr := parseDateFlexible(dateStr); derr == nil {
					datePtr = &dt
				}
			}

			// Build ClassProgress record
			cp := models.ClassProgress{
				FileName:       get("FileName"),
				FileID:         get("FileId"),
				SpreadsheetURL: get("SpreadsheetURL"),
				SheetTab:       get("SheetTab"),
				GroupID:        getIDPtrFromGroup(group),
				CourseID:       getIDPtrFromCourse(course),
				TeacherID:      teacherID,
				BookID:         bookID,
				Number:         parseIntPtr(get("No")),
				LessonPlan:     firstNonEmpty(get("LessonPlan"), get("LessionPlan")),
				Date:           datePtr,
				Hour:           parseIntPtr(get("Hour")),
				WarmUp:         get("WarmUp"),
				Topic:          get("Topic"),
				LastPage:       get("LastPage"),
				ProgressCheck:  firstNonEmpty(get("Progress check"), get("Progress")),
				Comment:        get("Comment"),
				GoalInfo:       firstNonEmpty(get("Goal + Infomation"), get("Goal + Information")),
				BookNameRaw:    bookRaw,
				Level:          level,
				CoursePath:     coursePath,
				TargetHours:    targetHours,
				SpecialHours:   specialHours,
				TotalHours:     totalHour,
				BranchRaw:      branchRaw,
			}

			// Prevent duplicate ClassProgress on re-import
			// Prefer key: (file_id, sheet_tab, number) OR (file_id, sheet_tab, date)
			// Fallback to (file_name, sheet_tab, number/date) or (group_id, number, date)
			var existingCP models.ClassProgress
			dupChecked := false
			if strings.TrimSpace(cp.FileID) != "" && strings.TrimSpace(cp.SheetTab) != "" {
				if cp.Number != nil {
					if err := tx.Where("file_id = ? AND sheet_tab = ? AND number = ?", cp.FileID, cp.SheetTab, *cp.Number).First(&existingCP).Error; err == nil {
						duplicateRows++
						skipped++
						continue
					}
					dupChecked = true
				} else if cp.Date != nil {
					if err := tx.Where("file_id = ? AND sheet_tab = ? AND date = ?", cp.FileID, cp.SheetTab, cp.Date).First(&existingCP).Error; err == nil {
						duplicateRows++
						skipped++
						continue
					}
					dupChecked = true
				}
			}
			if !dupChecked && strings.TrimSpace(cp.FileName) != "" && strings.TrimSpace(cp.SheetTab) != "" {
				if cp.Number != nil {
					if err := tx.Where("file_name = ? AND sheet_tab = ? AND number = ?", cp.FileName, cp.SheetTab, *cp.Number).First(&existingCP).Error; err == nil {
						duplicateRows++
						skipped++
						continue
					}
					dupChecked = true
				} else if cp.Date != nil {
					if err := tx.Where("file_name = ? AND sheet_tab = ? AND date = ?", cp.FileName, cp.SheetTab, cp.Date).First(&existingCP).Error; err == nil {
						duplicateRows++
						skipped++
						continue
					}
					dupChecked = true
				}
			}
			if !dupChecked && cp.GroupID != nil && cp.Number != nil && cp.Date != nil {
				if err := tx.Where("group_id = ? AND number = ? AND date = ?", *cp.GroupID, *cp.Number, cp.Date).First(&existingCP).Error; err == nil {
					duplicateRows++
					skipped++
					continue
				}
			}

			if err := tx.Create(&cp).Error; err != nil {
				errorsList = append(errorsList, fmt.Sprintf("row %d: %v", i+1, err))
				continue
			}
			created++
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":            true,
		"file_name":          fileHeader.Filename,
		"file_total_rows":    totalRows,
		"data_rows":          dataRows,
		"imported_rows":      created,
		"skipped_rows":       skipped,
		"duplicate_rows":     duplicateRows,
		"unique_names_total": len(uniqueNameKeys),
		"people_imported":    studentsCreated, // new Student records created
		"students_created":   studentsCreated,
		"students_reused":    studentsReused,
		"members_added":      membersAdded,
		"errors_count":       len(errorsList),
		"has_errors":         len(errorsList) > 0,
		"errors":             errorsList,
	})
}

// POST /api/import/class-progress/undo
// Body JSON: { file_id?: string, file_name?: string, spreadsheet_url?: string, dry_run?: bool=true, delete_orphans?: bool=true, include_students?: bool=false }
// Deletes ClassProgress rows matching any provided identifier and optionally cleans orphans in FK-safe order.
func (ic *ClassProgressImportController) Undo(c *fiber.Ctx) error {
	type reqT struct {
		FileID          string `json:"file_id"`
		FileName        string `json:"file_name"`
		SpreadsheetURL  string `json:"spreadsheet_url"`
		DryRun          *bool  `json:"dry_run"`
		DeleteOrphans   *bool  `json:"delete_orphans"`
		IncludeStudents *bool  `json:"include_students"`
	}
	var req reqT
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	deleteOrphans := true
	if req.DeleteOrphans != nil {
		deleteOrphans = *req.DeleteOrphans
	}
	includeStudents := false
	if req.IncludeStudents != nil {
		includeStudents = *req.IncludeStudents
	}

	db := database.DB

	// Build query for target ClassProgress rows
	q := db.Model(&models.ClassProgress{})
	filters := 0
	if strings.TrimSpace(req.FileID) != "" {
		q = q.Or("file_id = ?", req.FileID)
		filters++
	}
	if strings.TrimSpace(req.FileName) != "" {
		q = q.Or("file_name = ?", req.FileName)
		filters++
	}
	if strings.TrimSpace(req.SpreadsheetURL) != "" {
		q = q.Or("spreadsheet_url = ?", req.SpreadsheetURL)
		filters++
	}
	if filters == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "at least one of file_id, file_name, spreadsheet_url is required"})
	}

	// Collect target rows
	var cps []models.ClassProgress
	if err := q.Find(&cps).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if len(cps) == 0 {
		return c.JSON(fiber.Map{"success": true, "deleted": 0, "details": fiber.Map{"message": "no matching class progress"}})
	}

	// Gather related IDs
	groupIDs := map[uint]struct{}{}
	courseIDs := map[uint]struct{}{}
	bookIDs := map[uint]struct{}{}
	for _, cp := range cps {
		if cp.GroupID != nil {
			groupIDs[*cp.GroupID] = struct{}{}
		}
		if cp.CourseID != nil {
			courseIDs[*cp.CourseID] = struct{}{}
		}
		if cp.BookID != nil {
			bookIDs[*cp.BookID] = struct{}{}
		}
	}

	// Prepare summary
	summary := fiber.Map{
		"target_progress":  len(cps),
		"groups_involved":  len(groupIDs),
		"courses_involved": len(courseIDs),
		"books_involved":   len(bookIDs),
		"actions":          []string{},
	}

	if dryRun {
		acts := []string{"DELETE ClassProgress WHERE matches filters"}
		if deleteOrphans {
			acts = append(acts, "CLEANUP orphan Groups (no members, no schedules)")
			acts = append(acts, "CLEANUP orphan Courses (no groups)")
			acts = append(acts, "CLEANUP orphan Books (no class progress)")
		}
		if includeStudents {
			acts = append(acts, "OPTIONAL: Delete Students and Users with no memberships and no references")
		}
		summary["actions"] = acts
		return c.JSON(fiber.Map{"success": true, "dry_run": true, "summary": summary})
	}

	// Execute within a transaction in FK-safe order
	err := db.Transaction(func(tx *gorm.DB) error {
		// 1) Delete ClassProgress first
		if err := q.Unscoped().Delete(&models.ClassProgress{}).Error; err != nil {
			return err
		}

		if deleteOrphans {
			// 2) Cleanup Groups with no members and no schedules
			if len(groupIDs) > 0 {
				ids := keysUint(groupIDs)
				var groups []models.Group
				if err := tx.Where("id IN ?", ids).Find(&groups).Error; err != nil {
					return err
				}
				for _, g := range groups {
					var memberCount int64
					if err := tx.Model(&models.GroupMember{}).Where("group_id = ?", g.ID).Count(&memberCount).Error; err != nil {
						return err
					}
					var scheduleCount int64
					if err := tx.Model(&models.Schedules{}).Where("group_id = ?", g.ID).Count(&scheduleCount).Error; err != nil {
						return err
					}
					if memberCount == 0 && scheduleCount == 0 {
						// safe to delete group
						if err := tx.Unscoped().Delete(&models.Group{}, g.ID).Error; err != nil {
							return err
						}
					}
				}
			}

			// 3) Cleanup Courses with no groups
			if len(courseIDs) > 0 {
				cids := keysUint(courseIDs)
				var courses []models.Course
				if err := tx.Where("id IN ?", cids).Find(&courses).Error; err != nil {
					return err
				}
				for _, co := range courses {
					var grpCount int64
					if err := tx.Model(&models.Group{}).Where("course_id = ?", co.ID).Count(&grpCount).Error; err != nil {
						return err
					}
					if grpCount == 0 {
						if err := tx.Unscoped().Delete(&models.Course{}, co.ID).Error; err != nil {
							return err
						}
					}
				}
			}

			// 4) Cleanup Books with no ClassProgress referencing
			if len(bookIDs) > 0 {
				bids := keysUint(bookIDs)
				var books []models.Book
				if err := tx.Where("id IN ?", bids).Find(&books).Error; err != nil {
					return err
				}
				for _, b := range books {
					var cpCount int64
					if err := tx.Model(&models.ClassProgress{}).Where("book_id = ?", b.ID).Count(&cpCount).Error; err != nil {
						return err
					}
					if cpCount == 0 {
						if err := tx.Unscoped().Delete(&models.Book{}, b.ID).Error; err != nil {
							return err
						}
					}
				}
			}

			// 5) Optionally, cleanup Students and Users with no memberships or references
			if includeStudents {
				// Find students that are not in any group
				var lonelyStudents []models.Student
				if err := tx.
					Raw("SELECT s.* FROM students s LEFT JOIN group_members gm ON gm.student_id = s.id WHERE gm.id IS NULL").
					Scan(&lonelyStudents).Error; err != nil {
					return err
				}
				for _, s := range lonelyStudents {
					// Additional safety: ensure not referenced in schedules or other tables
					var anyRef int64
					// If add other references later, extend here
					if err := tx.Model(&models.GroupMember{}).Where("student_id = ?", s.ID).Count(&anyRef).Error; err != nil {
						return err
					}
					if anyRef == 0 {
						// delete student first
						if err := tx.Unscoped().Delete(&models.Student{}, s.ID).Error; err != nil {
							return err
						}
						// delete user if exists and has no teacher/student left
						if s.UserID != nil {
							var countSt int64
							if err := tx.Model(&models.Student{}).Where("user_id = ?", *s.UserID).Count(&countSt).Error; err != nil {
								return err
							}
							var countT int64
							if err := tx.Model(&models.Teacher{}).Where("user_id = ?", *s.UserID).Count(&countT).Error; err != nil {
								return err
							}
							if countSt == 0 && countT == 0 {
								if err := tx.Unscoped().Delete(&models.User{}, *s.UserID).Error; err != nil {
									return err
								}
							}
						}
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "dry_run": false, "summary": summary})
}

func readCSV(r io.Reader) ([][]string, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	var rows [][]string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

func readXLSX(path string) ([][]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Use first sheet
	sht := f.GetSheetName(0)
	if sht == "" {
		sht = "Sheet1"
	}
	data, err := f.GetRows(sht)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func buildColumnIndex(header []string) map[string]int {
	m := map[string]int{}
	for i, h := range header {
		key := strings.TrimSpace(h)
		m[key] = i
		// Normalize some keys
		if strings.EqualFold(key, "FileId") {
			m["FileId"] = i
		}
		if strings.EqualFold(key, "No") {
			m["No"] = i
		}
	}
	return m
}

func parseIntPtr(s string) *int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return &n
	}
	return nil
}

func firstNonEmpty(a string, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstNumber(s string) int {
	s = strings.TrimSpace(s)
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	return 0
}

// extractNumbersList finds all numeric tokens from a raw string like "1,3", "สาขา1-3"
// It returns unique numbers as strings in the order they appear.
func extractNumbersList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Normalize common separators to space
	repl := strings.NewReplacer(",", " ", "/", " ", "-", " ", "_", " ")
	s = repl.Replace(s)
	tokens := strings.Fields(s)
	seen := map[string]struct{}{}
	out := []string{}
	for _, t := range tokens {
		// pull digits from token
		digits := ""
		for _, ch := range t {
			if ch >= '0' && ch <= '9' {
				digits += string(ch)
			}
		}
		if digits == "" {
			continue
		}
		if _, ok := seen[digits]; ok {
			continue
		}
		seen[digits] = struct{}{}
		out = append(out, digits)
	}
	return out
}

// Helpers for extracting IDs from optional relations
func getIDPtrFromGroup(g *models.Group) *uint {
	if g == nil {
		return nil
	}
	id := g.ID
	return &id
}
func getIDPtrFromCourse(co *models.Course) *uint {
	if co == nil {
		return nil
	}
	id := co.ID
	return &id
}

func parseDateFlexible(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	// Common formats: 18/05/22, 26/09/24, 2022-05-18
	layouts := []string{"02/01/06", "02/01/2006", time.DateOnly, time.RFC3339}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			// If year is 2-digit, time.Parse makes it 20xx by Go's rules; acceptable
			return t, nil
		}
	}
	// Sometimes coming from Google sheets as URL date? try URL decode just in case
	if u, err := url.QueryUnescape(s); err == nil && u != s {
		return parseDateFlexible(u)
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %s", s)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "..", "_")
	return name
}

// Build a stable group name from file name and level; fallback to student names
func buildGroupName(fileName, level string, ths, ens []string) string {
	base := strings.TrimSpace(fileName)
	if base == "" {
		// Use first available student name
		var parts []string
		for _, s := range ths {
			if strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		for _, s := range ens {
			if strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		if len(parts) > 0 {
			base = strings.Join(parts, "/")
		} else {
			base = "Group"
		}
	}
	if strings.TrimSpace(level) != "" {
		base = fmt.Sprintf("%s/%s", base, strings.TrimSpace(level))
	}
	// Limit length
	if len(base) > 120 {
		base = base[:120]
	}
	return base
}

// resolveBranchByNumbers tries to map Branch field like "1,3" to an existing Branch ID by direct match or fallback to ONLINE code
func resolveBranchByNumbers(tx *gorm.DB, branchRaw string, fileNameHint string) uint {
	// Extract candidate numbers from Branch field or file name hint
	nums := extractNumbersList(branchRaw)
	if len(nums) == 0 {
		nums = extractNumbersList(fileNameHint)
	}

	if len(nums) > 0 {
		// Load branches once
		var branches []models.Branch
		_ = tx.Find(&branches).Error
		for _, n := range nums {
			// Prefer matching by name "Branch n" or Thai "สาขา n" (with/without space)
			for _, b := range branches {
				nameEn := strings.ToLower(b.NameEn)
				nameTh := strings.ToLower(b.NameTh)
				if strings.Contains(nameEn, "branch "+strings.ToLower(n)) ||
					strings.Contains(nameTh, "สาขา"+n) || strings.Contains(nameTh, "สาขา "+n) {
					return b.ID
				}
			}
		}
		// As a last resort, if number accidentally equals a real ID and above didn't match, try ID
		for _, n := range nums {
			if iv, err := strconv.Atoi(n); err == nil && iv > 0 {
				var b models.Branch
				if err := tx.First(&b, uint(iv)).Error; err == nil {
					return b.ID
				}
			}
		}
	}
	// try by code names
	candidates := []string{"ONLINE", "RMUTI", "MALL"}
	for _, code := range candidates {
		var b models.Branch
		if err := tx.Where("code = ?", code).First(&b).Error; err == nil {
			return b.ID
		}
	}
	// default to id=3 if exists
	var def models.Branch
	if err := tx.First(&def, uint(3)).Error; err == nil {
		return def.ID
	}
	// else first branch
	if err := tx.First(&def).Error; err == nil {
		return def.ID
	}
	return 0
}

func findOrCreateCourse(tx *gorm.DB, name string, branchID uint, level string) (*models.Course, error) {
	var c models.Course
	if err := tx.Where("name = ?", name).First(&c).Error; err == nil {
		return &c, nil
	}
	// fuzzy: try LIKE
	var like models.Course
	if err := tx.Where("name LIKE ?", "%"+name+"%").First(&like).Error; err == nil {
		return &like, nil
	}
	// create new
	// Generate a unique non-empty code to satisfy unique index on courses.code
	genCode := fmt.Sprintf("auto-%d", time.Now().UnixNano())
	nc := models.Course{Name: name, Code: genCode, BranchID: branchID, Level: level, Status: "active"}
	if err := tx.Create(&nc).Error; err != nil {
		return nil, err
	}
	return &nc, nil
}

func findTeacherClosest(tx *gorm.DB, key string) *models.Teacher {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	var t models.Teacher
	if err := tx.Where("nickname_en = ? OR nickname_th = ?", key, key).First(&t).Error; err == nil {
		return &t
	}
	// try LIKE
	var like models.Teacher
	if err := tx.Where("nickname_en LIKE ? OR nickname_th LIKE ?", "%"+key+"%", "%"+key+"%").First(&like).Error; err == nil {
		return &like
	}
	return nil
}

// findStudentByNicknames tries to find a student by exact Thai or English nickname
func findStudentByNicknames(tx *gorm.DB, th, en string) *models.Student {
	var s models.Student
	th = strings.TrimSpace(th)
	en = strings.TrimSpace(en)
	if th != "" && en != "" {
		if err := tx.Where("nickname_th = ? OR nickname_en = ?", th, en).First(&s).Error; err == nil {
			return &s
		}
	} else if th != "" {
		if err := tx.Where("nickname_th = ?", th).First(&s).Error; err == nil {
			return &s
		}
	} else if en != "" {
		if err := tx.Where("nickname_en = ?", en).First(&s).Error; err == nil {
			return &s
		}
	}
	return nil
}

// findOrCreateUserByUsername finds a user by username or creates one with defaults
func findOrCreateUserByUsername(tx *gorm.DB, username string, branchID uint, hashedPassword string) (models.User, error) {
	var user models.User
	if err := tx.Where("username = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Email left nil to avoid unique '' collisions
			user = models.User{Username: username, Password: hashedPassword, Role: "student", BranchID: branchID, Status: "active", Email: nil}
			if err := tx.Create(&user).Error; err != nil {
				return models.User{}, err
			}
		} else {
			return models.User{}, err
		}
	}
	return user, nil
}

// keysUint returns the keys of a map[uint]struct{} as a slice
func keysUint(m map[uint]struct{}) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
