# Error Format & Common Cases

Standard error:
{ "error": "message" }

Common:
- 401 Missing/Invalid Authorization header or token
- 403 Insufficient permissions (e.g., schedule create by non-admin)
- 400 Validation errors (invalid IDs, formats)
- 404 Resource not found
- 409 Conflicts (e.g., duplicate course code)
