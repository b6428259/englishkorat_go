package routes //nolint:goconst

import (
	"englishkorat_go/config"
	"englishkorat_go/controllers"
	"englishkorat_go/middleware"
	"englishkorat_go/services"
	notifsvc "englishkorat_go/services/notifications"
	"englishkorat_go/services/websocket"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App, wsHub *websocket.Hub, healthService *services.HealthService) {
	// Initialize controllers
	authController := &controllers.AuthController{}
	userController := &controllers.UserController{}
	courseController := &controllers.CourseController{}
	userInCourseController := &controllers.UserInCourseController{}
	branchController := &controllers.BranchController{}
	studentController := &controllers.StudentController{}
	teacherController := &controllers.TeacherController{}
	roomController := &controllers.RoomController{}
	notificationController := &controllers.NotificationController{}
	logController := &controllers.LogController{}
	scheduleController := &controllers.ScheduleController{}
	groupController := &controllers.GroupController{}
	classProgressImportController := &controllers.ClassProgressImportController{}
	scheduleImportController := &controllers.ScheduleImportController{}
	billsImportController := &controllers.BillsImportController{}
	billsController := &controllers.BillsController{}
	absenceController := &controllers.AbsenceController{}
	settingsController := controllers.NewSettingsController()
	healthController := controllers.NewHealthController(healthService)
	wsController := controllers.NewWebSocketController(wsHub)

	// localhost:3000/api/auth/login

	// API group
	api := app.Group("/api")

	// Comprehensive health check
	api.Get("/health", healthController.GetHealthStatus)

	// Public routes (no authentication required)
	public := api.Group("/public")

	// Courses - PUBLIC endpoint as required
	public.Get("/courses", courseController.GetCourses) //nolint:goconst
	public.Get("/courses/:id", courseController.GetCourse)
	public.Get("/courses/branch/:branch_id", courseController.GetCoursesByBranch)

	// Dev-only: public test endpoint to send a popup notification to a user (no auth)
	public.Post("/notifications/test", func(c *fiber.Ctx) error {
		if config.AppConfig == nil || config.AppConfig.AppEnv != "development" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not allowed"})
		}
		var req struct {
			UserID uint   `json:"user_id"`
			Title  string `json:"title"`
			Msg    string `json:"message"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if req.UserID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id required"})
		}
		title := req.Title
		if title == "" {
			title = "Test Popup"
		}
		msg := req.Msg
		if msg == "" {
			msg = "This is a test popup notification."
		}
		q := notifsvc.QueuedWithData(title, "ทดสอบป๊อปอัพ", msg, "นี่คือการแจ้งเตือนป๊อปอัพทดสอบ", "info", fiber.Map{"action": "test-popup"}, "popup", "normal")
		if err := notifsvc.NewService().EnqueueOrCreate([]uint{req.UserID}, q); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send"})
		}
		return c.JSON(fiber.Map{"message": "queued"})
	})

	// Student Registration - PUBLIC endpoints
	public.Post("/students/student-register", studentController.PublicRegisterStudent)
	public.Post("/students/new-register", studentController.NewPublicRegisterStudent) // New structured registration endpoint
	// Also expose at /api/students/student-register (no auth)
	api.Post("/students/student-register", studentController.PublicRegisterStudent)
	api.Post("/students/new-register", studentController.NewPublicRegisterStudent) // New structured registration endpoint

	// Also expose public courses directly under /api/courses for unauthenticated access
	// This ensures requests to /api/courses/ (with trailing slash) are handled.
	api.Get("/courses", courseController.GetCourses) //nolint:goconst
	api.Get("/courses/:id", courseController.GetCourse)
	api.Get("/courses/branch/:branch_id", courseController.GetCoursesByBranch)

	// Authentication routes (no middleware)
	auth := api.Group("/auth")
	auth.Post("/login", authController.Login)
	auth.Post("/reset-password-token", authController.ResetPasswordWithToken) // Public endpoint for token-based reset
	// Allow profile retrieval via /api/auth/profile using the same JWT middleware
	auth.Get("/profile", middleware.JWTMiddleware(), authController.GetProfile)

	// Protected routes (require authentication)
	protected := api.Group("/", middleware.JWTMiddleware())

	// Profile routes (authenticated users)
	protected.Get("/profile", authController.GetProfile)
	protected.Put("/profile/password", authController.ChangePassword)
	// Logout - blacklist token for 24 hours
	protected.Post("/auth/logout", authController.Logout)

	// Password reset routes (admin/owner only)
	passwordReset := protected.Group("/password-reset", middleware.RequireOwnerOrAdmin())
	passwordReset.Post("/generate-token", authController.GeneratePasswordResetToken)
	passwordReset.Post("/reset-by-admin", authController.ResetPasswordByAdmin)

	// User management routes
	users := protected.Group("/users")
	users.Get("/", middleware.RequireTeacherOrAbove(), userController.GetUsers)
	users.Get("/:id", middleware.RequireTeacherOrAbove(), userController.GetUser)
	users.Post("/", middleware.RequireOwnerOrAdmin(), authController.Register) // Use register from auth controller
	users.Put("/:id", middleware.RequireOwnerOrAdmin(), userController.UpdateUser)
	users.Delete("/:id", middleware.RequireOwnerOrAdmin(), userController.DeleteUser)
	users.Post("/:id/avatar", userController.UploadAvatar) // Users can upload their own avatar
	users.Get("/:id/settings", middleware.RequireOwnerOrAdmin(), settingsController.GetUserSettings)
	users.Put("/:id/settings", middleware.RequireOwnerOrAdmin(), settingsController.UpdateUserSettings)
	users.Post("/:id/settings/custom-sound", middleware.RequireOwnerOrAdmin(), settingsController.UploadUserCustomSound)

	// Course management routes (protected)
	courses := protected.Group("/courses")
	courses.Post("/", middleware.RequireOwnerOrAdmin(), courseController.CreateCourse)
	courses.Put("/:id", middleware.RequireOwnerOrAdmin(), courseController.UpdateCourse)
	courses.Delete("/:id", middleware.RequireOwnerOrAdmin(), courseController.DeleteCourse)
	// Assign users to course
	courses.Post("/:id/assignments", middleware.RequireOwnerOrAdmin(), userInCourseController.AssignUserToCourse)
	// Bulk assign users to course
	courses.Post("/:id/assignments/bulk", middleware.RequireOwnerOrAdmin(), userInCourseController.AssignUsersToCourseBulk)

	// Branch management routes
	branches := protected.Group("/branches")
	branches.Get("/", middleware.RequireTeacherOrAbove(), branchController.GetBranches)
	branches.Get("/:id", middleware.RequireTeacherOrAbove(), branchController.GetBranch)
	branches.Post("/", middleware.RequireOwnerOrAdmin(), branchController.CreateBranch)
	branches.Put("/:id", middleware.RequireOwnerOrAdmin(), branchController.UpdateBranch)
	branches.Delete("/:id", middleware.RequireOwnerOrAdmin(), branchController.DeleteBranch)

	// Student management routes
	students := protected.Group("/students")
	students.Get("/", middleware.RequireTeacherOrAbove(), studentController.GetStudents)
	students.Get("/:id", middleware.RequireTeacherOrAbove(), studentController.GetStudent)
	students.Post("/", middleware.RequireTeacherOrAbove(), studentController.CreateStudent)
	students.Put("/:id", middleware.RequireTeacherOrAbove(), studentController.UpdateStudent)
	students.Delete("/:id", middleware.RequireOwnerOrAdmin(), studentController.DeleteStudent)
	students.Get("/branch/:branch_id", middleware.RequireTeacherOrAbove(), studentController.GetStudentsByBranch) //nolint:goconst

	// New admin endpoints for the redesigned registration workflow
	students.Patch("/:id", middleware.RequireTeacherOrAbove(), studentController.UpdateStudentInfo)               // Complete student information
	students.Get("/by-status/:status", middleware.RequireTeacherOrAbove(), studentController.GetStudentsByStatus) // Filter by registration status
	students.Post("/:id/exam-scores", middleware.RequireTeacherOrAbove(), studentController.SetExamScores)        // Record exam scores

	// Backward/alternate path for Update as per docs
	api.Put("/v1/students/:id", middleware.JWTMiddleware(), middleware.RequireTeacherOrAbove(), studentController.UpdateStudent)

	// Teacher management routes
	teachers := protected.Group("/teachers")
	teachers.Get("/", middleware.RequireTeacherOrAbove(), teacherController.GetTeachers)
	teachers.Get("/:id", middleware.RequireTeacherOrAbove(), teacherController.GetTeacher)
	teachers.Post("/", middleware.RequireOwnerOrAdmin(), teacherController.CreateTeacher)
	teachers.Put("/:id", middleware.RequireOwnerOrAdmin(), teacherController.UpdateTeacher)
	teachers.Delete("/:id", middleware.RequireOwnerOrAdmin(), teacherController.DeleteTeacher)
	teachers.Get("/branch/:branch_id", middleware.RequireTeacherOrAbove(), teacherController.GetTeachersByBranch) //nolint:goconst
	teachers.Get("/specializations", teacherController.GetTeacherSpecializations)
	teachers.Get("/types", teacherController.GetTeacherTypes)

	// Room management routes
	rooms := protected.Group("/rooms")
	rooms.Get("/", middleware.RequireTeacherOrAbove(), roomController.GetRooms)
	rooms.Get("/:id", middleware.RequireTeacherOrAbove(), roomController.GetRoom)
	rooms.Post("/", middleware.RequireOwnerOrAdmin(), roomController.CreateRoom)
	rooms.Put("/:id", middleware.RequireOwnerOrAdmin(), roomController.UpdateRoom)
	rooms.Delete("/:id", middleware.RequireOwnerOrAdmin(), roomController.DeleteRoom)
	rooms.Get("/branch/:branch_id", middleware.RequireTeacherOrAbove(), roomController.GetRoomsByBranch) //nolint:goconst
	rooms.Get("/available", middleware.RequireTeacherOrAbove(), roomController.GetAvailableRooms)
	rooms.Patch("/:id/status", middleware.RequireTeacherOrAbove(), roomController.UpdateRoomStatus)

	// Notification management routes
	notifications := protected.Group("/notifications")
	notifications.Get("/", notificationController.GetNotifications)
	notifications.Get("/unread-count", notificationController.GetUnreadCount)
	notifications.Get("/stats", middleware.RequireOwnerOrAdmin(), notificationController.GetNotificationStats) //nolint:goconst
	notifications.Get("/:id", notificationController.GetNotification)
	notifications.Post("/", middleware.RequireOwnerOrAdmin(), notificationController.CreateNotification)
	notifications.Patch("/:id/read", notificationController.MarkAsRead)
	notifications.Patch("/mark-all-read", notificationController.MarkAllAsRead)
	notifications.Delete("/:id", notificationController.DeleteNotification)
	// Test endpoint: send popup notification for all scenarios (dev/testing)
	notifications.Get("/test/popup", notificationController.TestWebSocketPopup)

	// Log management routes (Admin/Owner only)
	logs := protected.Group("/logs", middleware.RequireOwnerOrAdmin())
	logs.Get("/", logController.GetLogs)
	logs.Get("/stats", logController.GetLogStats) //nolint:goconst
	logs.Get("/:id", logController.GetLog)
	logs.Delete("/old", logController.DeleteOldLogs)
	logs.Get("/export", logController.ExportLogs)
	logs.Post("/flush-cache", logController.FlushCachedLogs)

	// Schedule management routes
	schedules := protected.Group("/schedules")

	// Schedule CRUD operations
	schedules.Post("/", middleware.RequireOwnerOrAdmin(), scheduleController.CreateSchedule)
	schedules.Post("/preview", middleware.RequireOwnerOrAdmin(), scheduleController.PreviewSchedule)
	schedules.Post("/rooms/check-conflicts", middleware.RequireOwnerOrAdmin(), scheduleController.CheckRoomConflicts)
	schedules.Get("/", middleware.RequireOwnerOrAdmin(), scheduleController.GetSchedules)
	schedules.Get("/teachers", middleware.RequireTeacherOrAbove(), scheduleController.GetTeachersSchedules)
	// Alias singular path as requested
	schedules.Get("/teacher", middleware.RequireTeacherOrAbove(), scheduleController.GetTeachersSchedules)
	schedules.Get("/my", scheduleController.GetMySchedules)             // ดู schedule ของตัวเอง
	schedules.Patch("/:id/confirm", scheduleController.ConfirmSchedule) // ยืนยัน schedule
	schedules.Patch("/sessions/:id/confirm", scheduleController.ConfirmSession)

	// Session management
	schedules.Get("/:id/sessions", scheduleController.GetScheduleSessions)          // ดู sessions ของ schedule
	schedules.Get("/sessions/:id", scheduleController.GetSession)                   // ดูรายละเอียดของ session
	schedules.Patch("/sessions/:id/status", scheduleController.UpdateSessionStatus) // อัพเดทสถานะ session
	schedules.Post("/sessions/makeup", scheduleController.CreateMakeupSession)      // สร้าง makeup session
	// New: participant updates participation status for non-class schedules
	schedules.Patch("/:id/participants/me", scheduleController.UpdateMyParticipationStatus)
	// New: add a session into an existing schedule
	schedules.Post("/:id/sessions", scheduleController.AddSessionToSchedule)

	// Comment management
	schedules.Post("/comments", scheduleController.AddComment) // เพิ่ม comment
	schedules.Get("/comments", scheduleController.GetComments) // ดู comments

	// Calendar endpoint
	schedules.Get("/calendar", middleware.RequireTeacherOrAbove(), scheduleController.GetCalendarView)

	// Schedule detail (place after static paths to avoid collision with /teachers, /teacher, etc.)
	schedules.Get("/:id", scheduleController.GetScheduleDetail)

	// Group management routes
	groups := protected.Group("/groups")
	groups.Get("/", middleware.RequireTeacherOrAbove(), groupController.GetGroups)
	groups.Get("/:id", middleware.RequireTeacherOrAbove(), groupController.GetGroup)
	groups.Post("/", middleware.RequireOwnerOrAdmin(), groupController.CreateGroup)
	groups.Post("/:id/members", middleware.RequireOwnerOrAdmin(), groupController.AddMemberToGroup)
	groups.Delete("/:id/members/:student_id", middleware.RequireOwnerOrAdmin(), groupController.RemoveMemberFromGroup)
	groups.Patch("/:id/payment-status", middleware.RequireOwnerOrAdmin(), groupController.UpdateGroupPaymentStatus)

	// Import routes
	imports := protected.Group("/import", middleware.RequireOwnerOrAdmin())
	imports.Post("/class-progress", classProgressImportController.Import)
	imports.Post("/class-progress/undo", classProgressImportController.Undo)
	imports.Post("/schedules", scheduleImportController.Import)
	imports.Post("/bills", billsImportController.Import)

	// Bills management routes (financial data - restrict to owner/admin)
	bills := protected.Group("/bills", middleware.RequireOwnerOrAdmin())
	bills.Get("/", billsController.ListBills)
	bills.Get("/:id", billsController.GetBill)
	bills.Get("/by-transaction/:transactionId", billsController.GetByTransaction)
	bills.Get("/by-invoice/:invoice", billsController.GetByInvoice)
	bills.Post("/", billsController.CreateBill)
	bills.Patch("/:id", billsController.PatchBill)
	bills.Delete("/:id", billsController.DeleteBill)

	// Absence routes
	absences := protected.Group("/absences")
	absences.Post("/", middleware.RequireOwnerOrAdmin(), absenceController.CreateAbsence)        // นักเรียนส่งคำขอลา
	absences.Get("/", middleware.RequireTeacherOrAbove(), absenceController.GetAbsences)         // นักเรียน / ครู / แอดมิน ดูประวัติลา
	absences.Get("/:id", middleware.RequireOwnerOrAdmin(), absenceController.GetAbsencesByGroup) // ดูรายละเอียดการลา
	// absences.Patch("/:id/approve", middleware.RequireOwnerOrAdmin(), absenceController.ApproveAbsence) // อนุมัติ / ปฏิเสธการลา

	// Settings routes
	settings := protected.Group("/settings")
	settings.Get("/me", settingsController.GetMySettings)
	settings.Put("/me", settingsController.UpdateMySettings)
	settings.Post("/me/custom-sound", settingsController.UploadMyCustomSound)

	// WebSocket routes
	ws := protected.Group("/ws")
	ws.Get("/stats", middleware.RequireOwnerOrAdmin(), wsController.GetWebSocketStats) //nolint:goconst

	// WebSocket connection endpoint - use websocket upgrade middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if fiberws.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws", wsController.WebSocketHandler())
}

// SetupStaticRoutes configures static file serving
func SetupStaticRoutes(app *fiber.App) {
	// Serve static files if needed
	app.Static("/", "./public")
}
