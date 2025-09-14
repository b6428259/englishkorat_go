package routes

import (
	"englishkorat_go/controllers"
	"englishkorat_go/middleware"
	"englishkorat_go/services/websocket"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App, wsHub *websocket.Hub) {
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
	wsController := controllers.NewWebSocketController(wsHub)

	// API group
	api := app.Group("/api")

	// Public routes (no authentication required)
	public := api.Group("/public")

	// Courses - PUBLIC endpoint as required
	public.Get("/courses", courseController.GetCourses)
	public.Get("/courses/:id", courseController.GetCourse)
	public.Get("/courses/branch/:branch_id", courseController.GetCoursesByBranch)

	// Student Registration - PUBLIC endpoints
	public.Post("/students/student-register", studentController.PublicRegisterStudent)
	public.Post("/students/new-register", studentController.NewPublicRegisterStudent) // New structured registration endpoint
	// Also expose at /api/students/student-register (no auth)
	api.Post("/students/student-register", studentController.PublicRegisterStudent)
	api.Post("/students/new-register", studentController.NewPublicRegisterStudent) // New structured registration endpoint

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
	students.Get("/branch/:branch_id", middleware.RequireTeacherOrAbove(), studentController.GetStudentsByBranch)

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
	teachers.Get("/branch/:branch_id", middleware.RequireTeacherOrAbove(), teacherController.GetTeachersByBranch)
	teachers.Get("/specializations", teacherController.GetTeacherSpecializations)
	teachers.Get("/types", teacherController.GetTeacherTypes)

	// Room management routes
	rooms := protected.Group("/rooms")
	rooms.Get("/", middleware.RequireTeacherOrAbove(), roomController.GetRooms)
	rooms.Get("/:id", middleware.RequireTeacherOrAbove(), roomController.GetRoom)
	rooms.Post("/", middleware.RequireOwnerOrAdmin(), roomController.CreateRoom)
	rooms.Put("/:id", middleware.RequireOwnerOrAdmin(), roomController.UpdateRoom)
	rooms.Delete("/:id", middleware.RequireOwnerOrAdmin(), roomController.DeleteRoom)
	rooms.Get("/branch/:branch_id", middleware.RequireTeacherOrAbove(), roomController.GetRoomsByBranch)
	rooms.Get("/available", middleware.RequireTeacherOrAbove(), roomController.GetAvailableRooms)
	rooms.Patch("/:id/status", middleware.RequireTeacherOrAbove(), roomController.UpdateRoomStatus)

	// Notification management routes
	notifications := protected.Group("/notifications")
	notifications.Get("/", notificationController.GetNotifications)
	notifications.Get("/unread-count", notificationController.GetUnreadCount)
	notifications.Get("/stats", middleware.RequireOwnerOrAdmin(), notificationController.GetNotificationStats)
	notifications.Get("/:id", notificationController.GetNotification)
	notifications.Post("/", middleware.RequireOwnerOrAdmin(), notificationController.CreateNotification)
	notifications.Patch("/:id/read", notificationController.MarkAsRead)
	notifications.Patch("/mark-all-read", notificationController.MarkAllAsRead)
	notifications.Delete("/:id", notificationController.DeleteNotification)

	// Log management routes (Admin/Owner only)
	logs := protected.Group("/logs", middleware.RequireOwnerOrAdmin())
	logs.Get("/", logController.GetLogs)
	logs.Get("/stats", logController.GetLogStats)
	logs.Get("/:id", logController.GetLog)
	logs.Delete("/old", logController.DeleteOldLogs)
	logs.Get("/export", logController.ExportLogs)
	logs.Post("/flush-cache", logController.FlushCachedLogs)

	// Schedule management routes
	schedules := protected.Group("/schedules")

	// Schedule CRUD operations
	schedules.Post("/", middleware.RequireOwnerOrAdmin(), scheduleController.CreateSchedule)
	schedules.Get("/", middleware.RequireOwnerOrAdmin(), scheduleController.GetSchedules)
	schedules.Get("/teachers", middleware.RequireTeacherOrAbove(), scheduleController.GetTeachersSchedules)
	// Alias singular path as requested
	schedules.Get("/teacher", middleware.RequireTeacherOrAbove(), scheduleController.GetTeachersSchedules)
	schedules.Get("/my", scheduleController.GetMySchedules)             // ดู schedule ของตัวเอง
	schedules.Patch("/:id/confirm", scheduleController.ConfirmSchedule) // ยืนยัน schedule

	// Session management
	schedules.Get("/:id/sessions", scheduleController.GetScheduleSessions)          // ดู sessions ของ schedule
	schedules.Patch("/sessions/:id/status", scheduleController.UpdateSessionStatus) // อัพเดทสถานะ session
	schedules.Post("/sessions/makeup", scheduleController.CreateMakeupSession)      // สร้าง makeup session

	// Comment management
	schedules.Post("/comments", scheduleController.AddComment) // เพิ่ม comment
	schedules.Get("/comments", scheduleController.GetComments) // ดู comments

	// Calendar endpoint
	schedules.Get("/calendar", middleware.RequireTeacherOrAbove(), scheduleController.GetCalendarView)

	// WebSocket routes
	ws := protected.Group("/ws")
	ws.Get("/stats", middleware.RequireOwnerOrAdmin(), wsController.GetWebSocketStats)

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
