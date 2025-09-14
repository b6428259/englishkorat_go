# English Korat Go Backend

## Development Environment Setup

### Prerequisites
- Go 1.24.6 or higher
- MySQL 8.0 or higher
- Redis 6.0 or higher

### Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd englishkorat_go
```

2. Install dependencies:
```bash
go mod tidy
```

3. Copy environment configuration:
```bash
cp .env.example .env
```

4. Configure your `.env` file with appropriate values

5. Run the application:
```bash
go run main.go
```

### Development with Tunnel (EC2)

Use the PowerShell script to establish tunnel connection:
```powershell
.\start-dev.ps1
```

### Seeding the Database

Seeding is intentionally disabled during normal application startup to avoid
accidental data loss. To populate the database with seed data, run the
PowerShell seeder script which is the supported and documented method:

```powershell
./start-seed.ps1
```

Do NOT run seeding automatically in production or via `go run main.go`.
The Makefile `seed` target has been deprecated and now only points to the
PowerShell script.

### Skipping automatic migrations during development

To speed up local startup and avoid schema checks or ALTER statements during
development, you can set the `SKIP_MIGRATE` environment variable to `true`.
The `start-dev.ps1` script sets this variable for the spawned process so
automatic migrations are skipped when you run the app via the tunnel script.

Example (PowerShell):
```powershell
$env:SKIP_MIGRATE = "true"
go run main.go
```

### Project Structure

```
englishkorat_go/
├── config/          # Configuration files
├── controllers/     # HTTP request handlers
├── middleware/      # Middleware functions
├── models/          # Database models
├── routes/          # Route definitions
├── services/        # Business logic
├── utils/           # Utility functions
├── storage/         # File storage handling
├── logs/            # Application logs
├── database/        # Database related files
│   └── seeders/     # Database seeders
├── .env.example     # Environment configuration template
├── .env             # Environment configuration (ignored by git)
├── go.mod           # Go module file
├── go.sum           # Go dependencies
└── main.go          # Application entry point
```

### API Endpoints

#### Authentication
- `POST /api/auth/login` - User login
- `POST /api/auth/register` - User registration (admin only)

#### Public Endpoints
- `GET /api/courses` - Get all courses (public)

#### Protected Endpoints (require authentication)
- `GET /api/users` - Get users
- `POST /api/users` - Create user
- `PUT /api/users/:id` - Update user
- `DELETE /api/users/:id` - Delete user

[Similar patterns for branches, students, teachers, rooms, etc.]

### Roles and Permissions

- **Owner**: Full system access
- **Admin**: Full system access except owner management
- **Teacher**: Access to teaching-related features
- **Student**: Access to learning-related features

### Features

- JWT-based authentication
- Role-based access control
- Automatic database migration
- Bilingual support (Thai/English)
- Image upload with WebP conversion
- S3 bucket integration
- Comprehensive logging
- Real-time notifications
- Redis caching