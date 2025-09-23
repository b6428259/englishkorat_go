# WebSocket Message Guide

This guide documents the server->client WebSocket payloads your frontend should handle. Each pattern includes two sample messages to help you build robust routing and UI behavior.

## Connect
- URL: `ws(s)://<host>/ws?token=<JWT>`
- Auth: JWT in `token` query param (same token used for REST)
- Envelope: Messages are JSON objects.
- Admin stats (HTTP, JWT required, owner/admin): `GET /api/ws/stats`
- Dev-only test (HTTP, no auth, APP_ENV=development): `POST /api/public/notifications/test` with body `{ "user_id": 1, "title?": "...", "message?": "..." }`

Top-level envelope you will see most commonly:
- `{ "type": "notification", "data": NotificationDTO }`

Where `NotificationDTO` includes:
- `id, created_at, updated_at, user_id, title, title_th, message, message_th, type, channels[], read, read_at`
- `data` (optional): structured payload with deep-link/action
- `user, branch, sender, recipient` (compact objects)

Notes:
- `channels`: ["normal"|"popup"|"line"]. Multiple allowed. Default is ["normal"].
- `data.link`: `{ href: string, method: string }` when present.
- `data.action`: semantic string to drive UI routing.

Security:
- WebSocket token is validated server-side against active users; inactive/invalid tokens are rejected.
- Cross-origin is allowed in development; tighten origin checks in production if needed.

### Using data.link.href with your base_url

- The `data.link.href` field is a backend-relative API path. Your frontend should call it as: `{{base_url}} + href`.
- Examples of `base_url`:
  - Production: `https://api.yourdomain.com`
  - Development: `http://localhost:3000`
- Example: If the message contains `"link": { "href": "/api/schedules/sessions/2051", "method": "GET" }`, your frontend should request `{{base_url}}/api/schedules/sessions/2051` with the same JWT you use for REST.
- If `method` is omitted, default to `GET`.
- If the server ever returns an absolute URL in `href`, you can use it directly without prefixing `base_url`.

Minimal helper (JS/TS):

```js
async function executeLink(link, jwt, baseUrl) {
  const method = (link?.method || 'GET').toUpperCase();
  const href = link?.href || '';
  if (!href) throw new Error('Missing link.href');
  const url = href.startsWith('http') ? href : `${baseUrl}${href}`;
  const res = await fetch(url, {
    method,
    headers: {
      'Authorization': `Bearer ${jwt}`,
      'Content-Type': 'application/json'
    }
  });
  if (!res.ok) throw new Error(`Request failed: ${res.status}`);
  return res.headers.get('content-type')?.includes('application/json') ? res.json() : res.text();
}
```

---

## Pattern A: New Session Created (popup + normal)
Actionable popup inviting the user to review/confirm the session.

Example 1:
{
  "type": "notification",
  "data": {
    "id": 4123,
    "created_at": "2025-09-23T10:15:00Z",
    "user_id": 7,
    "title": "New session scheduled",
    "message": "A new session for schedule 'GE-Conversation' is scheduled at 2025-09-24 14:00.",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/sessions/2051", "method": "GET" },
      "action": "confirm-session",
      "session_id": 2051,
      "schedule_id": 330
    }
  }
}

Example 2:
{
  "type": "notification",
  "data": {
    "id": 4124,
    "created_at": "2025-09-23T10:16:00Z",
    "user_id": 9,
    "title": "New session scheduled",
    "message": "A new session for schedule 'IELTS Prep' is scheduled at 2025-09-24 09:30.",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/sessions/2052", "method": "GET" },
      "action": "confirm-session",
      "session_id": 2052,
      "schedule_id": 331
    }
  }
}

Handling:
- Show a modal/popup (channels includes "popup").
- Click-through should GET `data.link.href` to load details.
- Provide confirm CTA → PATCH `/api/schedules/sessions/{id}/confirm` when applicable.

---

## Pattern B: Upcoming Session Reminder (popup + normal)
Sent at 5, 30, 60 minutes before start.

Example 1:
{
  "type": "notification",
  "data": {
    "id": 4531,
    "title": "Upcoming Class",
    "message": "Your class 'GE-Conversation' will start in 30 minutes at 14:00",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/sessions/2051", "method": "GET" },
      "action": "open-session",
      "session_id": 2051,
      "schedule_id": 330,
      "reminder_before_minutes": 30
    }
  }
}

Example 2:
{
  "type": "notification",
  "data": {
    "id": 4532,
    "title": "Upcoming Class",
    "message": "Your class 'IELTS Prep' will start in 5 minutes at 09:30",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/sessions/2052", "method": "GET" },
      "action": "open-session",
      "session_id": 2052,
      "schedule_id": 331,
      "reminder_before_minutes": 5
    }
  }
}

Handling:
- If popup, use a compact reminder modal with quick open button.
- Deep-link to session detail, allow confirm if not already confirmed.

---

## Pattern C: Schedule Invitation (popup + normal)
Invite a user to a non-class schedule (event/appointment).

Example 1:
{
  "type": "notification",
  "data": {
    "id": 4610,
    "title": "Schedule invitation",
    "message": "You were invited to schedule: Parent Meeting.",
    "type": "info",
  "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/740", "method": "GET" },
      "action": "confirm-participation",
      "schedule_id": 740
    }
  }
}

Example 2:
{
  "type": "notification",
  "data": {
    "id": 4611,
    "title": "Schedule invitation",
    "message": "You were invited to schedule: Counseling.",
    "type": "info",
  "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/741", "method": "GET" },
      "action": "confirm-participation",
      "schedule_id": 741
    }
  }
}

Handling:
- Show a popup (channels includes "popup") and also add to the list/notification center.
- Offer confirm/decline via PATCH `/api/schedules/:id/participants/me`.

---

## Pattern D: Teacher Assigned to New Class (normal)
Notify default teacher when a class schedule is created.

Example 1:
{
  "type": "notification",
  "data": {
    "id": 4720,
    "title": "New Schedule Assignment",
    "message": "You have been assigned to schedule: GE-Conversation. Please review your sessions.",
    "type": "info",
    "channels": ["normal"],
    "data": {
      "link": { "href": "/api/schedules/800/sessions", "method": "GET" },
      "action": "review-schedule",
      "schedule_id": 800
    }
  }
}

Example 2:
{
  "type": "notification",
  "data": {
    "id": 4721,
    "title": "New Schedule Assignment",
    "message": "You have been assigned to schedule: IELTS Prep. Please review your sessions.",
    "type": "info",
    "channels": ["normal"],
    "data": {
      "link": { "href": "/api/schedules/801/sessions", "method": "GET" },
      "action": "review-schedule",
      "schedule_id": 801
    }
  }
}

Handling:
- Non-blocking toast with CTA to view sessions list.

---

## Pattern E: Daily Schedule Reminder (popup + normal)
Summary of today’s sessions per user.

Example 1:
{
  "type": "notification",
  "data": {
    "id": 4800,
    "title": "Daily Schedule Reminder",
    "message": "Today's schedule:\n- GE-Conversation at 14:00\n- IELTS Prep at 16:00\n",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "action": "open-today-schedule"
    }
  }
}

Example 2:
{
  "type": "notification",
  "data": {
    "id": 4801,
    "title": "Daily Schedule Reminder",
    "message": "ตารางเรียนวันนี้:\n- Conversation A เวลา 09:00\n- Writing B เวลา 13:00\n",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "action": "open-today-schedule"
    }
  }
}

Handling:
- Show popup with summary; take user to today view.

---

## Pattern F: Missed Session Alert (popup + normal)
Admin/Owner alert when a session is marked no-show.

Example 1:
{
  "type": "notification",
  "data": {
    "id": 4900,
    "title": "Missed Session Alert",
    "message": "Session 'GE-Conversation' on 2025-09-22 was missed (no-show)",
    "type": "warning",
    "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/sessions/2051", "method": "GET" },
      "action": "review-missed-session",
      "session_id": 2051,
      "schedule_id": 330
    }
  }
}

Example 2:
{
  "type": "notification",
  "data": {
    "id": 4901,
    "title": "Missed Session Alert",
    "message": "Session 'IELTS Prep' on 2025-09-20 was missed (no-show)",
    "type": "warning",
    "channels": ["popup", "normal"],
    "data": {
      "link": { "href": "/api/schedules/sessions/2052", "method": "GET" },
      "action": "review-missed-session",
      "session_id": 2052,
      "schedule_id": 331
    }
  }
}

Handling:
- Priority alert for admins; deep-link to details to follow up.

---

## Client handling snippet (JS)

```js
const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?token=${jwt}`);
ws.onmessage = (evt) => {
  const msg = JSON.parse(evt.data);
  if (msg.type === 'notification') {
    const n = msg.data;
    const channels = n.channels || ['normal'];
    const payload = n.data || {};

    // Always show as a toast or list item
    showToast(n.title, n.message);

    // Popup behavior
    if (channels.includes('popup')) {
      showPopup({ title: n.title, message: n.message, action: payload.action, link: payload.link });
    }

    // Route by action
    switch (payload.action) {
      case 'confirm-session':
      case 'open-session':
      case 'review-missed-session':
        if (payload.link) fetchAndOpen(payload.link.href);
        break;
      case 'confirm-participation':
        if (payload.link) fetchAndOpen(payload.link.href);
        break;
      case 'review-schedule':
        if (payload.link) fetchAndOpen(payload.link.href);
        break;
      case 'open-today-schedule':
        openTodayView();
        break;
      default:
        // no-op
    }
  }
};
```

## Tips
- Always check `channels` to decide UI weight.
- Prefer `data.link.href` for follow-ups; method is typically GET.
- Some messages may omit `data` (treat as informational only).
