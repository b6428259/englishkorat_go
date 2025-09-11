# Roles & Permissions

Roles: owner > admin > teacher > student

Middleware:
- RequireOwnerOrAdmin() — only owner/admin
- RequireTeacherOrAbove() — teacher/admin/owner

Key rules:
- Admin cannot reset owner password
- Schedule create: owner/admin only
- Room and Course create/update/delete: owner/admin only
- Teachers can only modify sessions for schedules they are assigned to
