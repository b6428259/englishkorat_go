package routes

import (
	"englishkorat_go/controllers"
	"englishkorat_go/middleware"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App) {
	// Initialize controllers
	authController := &controllers.AuthController{}
	userController := &controllers.UserController{}
	courseController := &controllers.CourseController{}
	branchController := &controllers.BranchController{}
	studentController := &controllers.StudentController{}
	teacherController := &controllers.TeacherController{}
	roomController := &controllers.RoomController{}
	notificationController := &controllers.NotificationController{}

	// API group
	api := app.Group("/api")

	// Public routes (no authentication required)
	public := api.Group("/public")
	
	// Courses - PUBLIC endpoint as required
	public.Get("/courses", courseController.GetCourses)
	public.Get("/courses/:id", courseController.GetCourse)
	public.Get("/courses/branch/:branch_id", courseController.GetCoursesByBranch)

	// Authentication routes (no middleware)
	auth := api.Group("/auth")
	auth.Post("/login", authController.Login)

	// Protected routes (require authentication)
	protected := api.Group("/", middleware.JWTMiddleware())

	// Profile routes (authenticated users)
	protected.Get("/profile", authController.GetProfile)
	protected.Put("/profile/password", authController.ChangePassword)

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
}

// SetupStaticRoutes configures static file serving
func SetupStaticRoutes(app *fiber.App) {
	// Serve static files if needed
	app.Static("/", "./public")
}