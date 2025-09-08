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

	// Additional protected endpoints will be added here for:
	// - Students 
	// - Teachers
	// - Rooms
	// - Notifications
	// - Activity logs
}

// SetupStaticRoutes configures static file serving
func SetupStaticRoutes(app *fiber.App) {
	// Serve static files if needed
	app.Static("/", "./public")
}