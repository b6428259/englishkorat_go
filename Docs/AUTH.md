# Authentication

Base path: `/api/auth`

## POST /login
- Body: { username, password }
- Response: { message, token, user }

Example response:
{
  "message": "Login successful",
  "token": "<jwt>",
  "user": { "id": 1, "username": "admin", "role": "admin", "branch_id": 1 }
}

## GET /api/profile
- Requires Authorization: Bearer <token>
- Returns current user profile with branch

## PUT /api/profile/password
- Body: { current_password, new_password }

## Password reset
- POST /api/password-reset/generate-token (owner/admin)
- POST /api/password-reset/reset-by-admin (owner/admin)
- POST /api/auth/reset-password-token (public)
