# Postman

Use the provided collections in the repo (Password Reset collection) or create a new one:

Environment vars:
- base_url: http://localhost:8080
- auth_token: set from login response token

Typical flow:
1. Auth: POST /api/auth/login -> save token to env
2. Courses: list public, then create/update with admin
3. Assignments: bulk assign users to course
4. Schedules: create schedule; teacher confirms; sessions operations
