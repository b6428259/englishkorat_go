# WebSocket Test Notifications

## Overview

This document describes how to test WebSocket popup notifications using the test endpoint. The endpoint allows developers to trigger various notification scenarios without needing to simulate actual system events.

## Endpoint

```
GET /api/notifications/test/popup
```

**Authentication:** Required (JWT token in Authorization header)

**Query Parameters:**
- `user_id` (optional): Target user ID to receive the notification. If not provided, sends to the authenticated user.
- `case` (optional): Test scenario to trigger. Default: `basic`

## Available Test Cases

You can specify test cases using either the name or numeric shortcut:

| Case Name | Shortcut | Description |
|-----------|----------|-------------|
| `basic` | `1` | Basic info popup notification |
| `schedule` | `2` | Schedule reminder (15 minutes before class) |
| `warning` | `3` | Schedule conflict warning |
| `success` | `4` | Success message (schedule approved) |
| `error` | `5` | Error notification (upload failed) |
| `normal_only` | `6` | Normal channel only (no popup) |
| `daily_reminder` | `7` | Daily schedule reminder |
| `payment_due` | `8` | Payment due warning |
| `makeup_session` | `9` | Makeup session scheduled |
| `absence_approved` | `10` | Absence request approved |
| `custom_sound` | `11` | Test custom sound (requires custom sound uploaded) |
| `long_message` | `12` | Long message test |
| `invitation` | `13` | Schedule invitation (requires response) |

## Usage Examples

### Test for Yourself

```bash
# Send basic notification to yourself
curl -X GET "http://localhost:3000/api/notifications/test/popup" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# Test schedule reminder
curl -X GET "http://localhost:3000/api/notifications/test/popup?case=schedule" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# Use numeric shortcut
curl -X GET "http://localhost:3000/api/notifications/test/popup?case=2" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### Test for Another User (Admin/Owner Only)

```bash
# Send notification to specific user
curl -X GET "http://localhost:3000/api/notifications/test/popup?user_id=123&case=warning" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Response Format

```json
{
  "message": "Test notification sent via WebSocket",
  "target_user": 123,
  "username": "john.doe",
  "test_case": "schedule",
  "description": "Schedule reminder (15 min)",
  "timestamp": "2025-01-16T10:30:00Z"
}
```

## Frontend Integration

### 1. WebSocket Connection

First, establish a WebSocket connection to receive notifications:

```javascript
// Connect to WebSocket
const token = localStorage.getItem('jwt_token');
const ws = new WebSocket(`ws://localhost:3000/ws?token=${token}`);

ws.onopen = () => {
  console.log('WebSocket connected');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
  
  // Handle notification
  if (data.notification) {
    handleNotification(data.notification, data.settings);
  }
};

ws.onerror = (error) => {
  console.error('WebSocket error:', error);
};

ws.onclose = () => {
  console.log('WebSocket closed');
};
```

### 2. Notification Handler

Handle incoming notifications and display popups:

```javascript
function handleNotification(notification, settings) {
  // Check if it's a popup notification
  if (notification.channels && notification.channels.includes('popup')) {
    showPopup(notification);
  }
  
  // Play notification sound if enabled
  if (settings && settings.enable_notification_sound) {
    playNotificationSound(notification, settings);
  }
}

function showPopup(notification) {
  // Create popup element
  const popup = document.createElement('div');
  popup.className = 'notification-popup';
  
  // Build action buttons based on notification type
  let actionButtons = '';
  if (notification.data?.requires_response) {
    // Invitation with Accept/Decline buttons
    actionButtons = `
      <div class="action-buttons">
        <button class="btn-accept" onclick="handleInvitationResponse(${notification.data.schedule_id}, 'accept')">Accept</button>
        <button class="btn-decline" onclick="handleInvitationResponse(${notification.data.schedule_id}, 'decline')">Decline</button>
      </div>
    `;
  } else if (notification.data?.action_label) {
    // Single action button
    actionButtons = `<button onclick="handleAction('${notification.data.action}')">${notification.data.action_label}</button>`;
  }
  
  popup.innerHTML = `
    <div class="popup-header ${notification.type}">
      <h4>${notification.title}</h4>
      <button onclick="closePopup(this)">×</button>
    </div>
    <div class="popup-body">
      <p>${notification.message}</p>
      ${actionButtons}
    </div>
  `;
  
  document.body.appendChild(popup);
  
  // Auto-close after 5 seconds (unless requires response)
  if (!notification.data?.requires_response) {
    setTimeout(() => popup.remove(), 5000);
  }
}

function handleInvitationResponse(scheduleId, response) {
  // Call API to accept/decline invitation
  fetch(`/api/schedules/${scheduleId}/participants/me`, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
    },
    body: JSON.stringify({
      participation_status: response === 'accept' ? 'confirmed' : 'declined'
    })
  })
  .then(response => response.json())
  .then(data => {
    console.log('Invitation response:', data);
    // Close the popup
    event.target.closest('.notification-popup').remove();
  })
  .catch(error => console.error('Error responding to invitation:', error));
}

function playNotificationSound(notification, settings) {
  let soundUrl = null;
  
  // Use custom sound if available and preference is set
  if (settings.notification_sound === 'custom' && settings.custom_sound_url) {
    soundUrl = settings.custom_sound_url;
  } else if (settings.notification_sound === 'default') {
    soundUrl = '/sounds/default-notification.mp3';
  }
  
  if (soundUrl) {
    const audio = new Audio(soundUrl);
    audio.play().catch(err => console.error('Failed to play sound:', err));
  }
}
```

### 3. Test Page Example

Create a simple test page to trigger notifications:

```html
<!DOCTYPE html>
<html>
<head>
  <title>WebSocket Notification Test</title>
  <style>
    .test-controls { padding: 20px; }
    .test-button { margin: 5px; padding: 10px 20px; }
    .notification-popup {
      position: fixed;
      top: 20px;
      right: 20px;
      width: 350px;
      background: white;
      border-radius: 8px;
      box-shadow: 0 4px 12px rgba(0,0,0,0.15);
      animation: slideIn 0.3s ease-out;
      z-index: 9999;
    }
    .popup-header {
      padding: 15px;
      border-radius: 8px 8px 0 0;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .popup-header.info { background: #3498db; color: white; }
    .popup-header.success { background: #2ecc71; color: white; }
    .popup-header.warning { background: #f39c12; color: white; }
    .popup-header.error { background: #e74c3c; color: white; }
    .popup-body { padding: 15px; }
    .action-buttons { margin-top: 10px; display: flex; gap: 10px; }
    .btn-accept { background: #2ecc71; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; }
    .btn-decline { background: #e74c3c; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; }
    .btn-accept:hover { background: #27ae60; }
    .btn-decline:hover { background: #c0392b; }
    @keyframes slideIn {
      from { transform: translateX(400px); opacity: 0; }
      to { transform: translateX(0); opacity: 1; }
    }
  </style>
</head>
<body>
  <div class="test-controls">
    <h2>WebSocket Notification Test</h2>
    <p>Connection Status: <span id="status">Disconnected</span></p>
    
    <h3>Test Cases</h3>
    <button class="test-button" onclick="testNotification('basic')">1. Basic Info</button>
    <button class="test-button" onclick="testNotification('schedule')">2. Schedule Reminder</button>
    <button class="test-button" onclick="testNotification('warning')">3. Conflict Warning</button>
    <button class="test-button" onclick="testNotification('success')">4. Success Message</button>
    <button class="test-button" onclick="testNotification('error')">5. Error Notification</button>
    <button class="test-button" onclick="testNotification('normal_only')">6. Normal Only (no popup)</button>
    <button class="test-button" onclick="testNotification('daily_reminder')">7. Daily Reminder</button>
    <button class="test-button" onclick="testNotification('payment_due')">8. Payment Due</button>
    <button class="test-button" onclick="testNotification('makeup_session')">9. Makeup Session</button>
    <button class="test-button" onclick="testNotification('absence_approved')">10. Absence Approved</button>
    <button class="test-button" onclick="testNotification('custom_sound')">11. Custom Sound</button>
    <button class="test-button" onclick="testNotification('long_message')">12. Long Message</button>
    <button class="test-button" onclick="testNotification('invitation')">13. Invitation</button>
  </div>

  <script>
    const token = localStorage.getItem('jwt_token');
    const ws = new WebSocket(`ws://localhost:3000/ws?token=${token}`);
    
    ws.onopen = () => {
      document.getElementById('status').textContent = 'Connected';
      document.getElementById('status').style.color = 'green';
    };
    
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      console.log('Received:', data);
      
      if (data.notification) {
        handleNotification(data.notification, data.settings);
      }
    };
    
    ws.onclose = () => {
      document.getElementById('status').textContent = 'Disconnected';
      document.getElementById('status').style.color = 'red';
    };
    
    function testNotification(testCase) {
      fetch(`/api/notifications/test/popup?case=${testCase}`, {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      })
      .then(response => response.json())
      .then(data => console.log('Test triggered:', data))
      .catch(error => console.error('Error:', error));
    }
    
    function handleNotification(notification, settings) {
      if (notification.channels && notification.channels.includes('popup')) {
        showPopup(notification);
      }
      
      if (settings && settings.enable_notification_sound) {
        playNotificationSound(notification, settings);
      }
    }
    
    function showPopup(notification) {
      const popup = document.createElement('div');
      popup.className = 'notification-popup';
      popup.innerHTML = `
        <div class="popup-header ${notification.type}">
          <h4>${notification.title}</h4>
          <button onclick="this.closest('.notification-popup').remove()">×</button>
        </div>
        <div class="popup-body">
          <p>${notification.message}</p>
        </div>
      `;
      
      document.body.appendChild(popup);
      setTimeout(() => popup.remove(), 5000);
    }
    
    function playNotificationSound(notification, settings) {
      let soundUrl = null;
      
      if (settings.notification_sound === 'custom' && settings.custom_sound_url) {
        soundUrl = settings.custom_sound_url;
      } else if (settings.notification_sound === 'default') {
        soundUrl = '/sounds/default-notification.mp3';
      }
      
      if (soundUrl) {
        const audio = new Audio(soundUrl);
        audio.play().catch(err => console.error('Failed to play sound:', err));
      }
    }
  </script>
</body>
</html>
```

## Test Workflow

1. **Open Test Page**: Load the HTML test page in your browser
2. **Connect WebSocket**: Page automatically connects when loaded
3. **Check Connection**: Verify "Connected" status is green
4. **Click Test Button**: Click any test case button (e.g., "2. Schedule Reminder")
5. **API Call Made**: Button triggers GET request to `/api/notifications/test/popup?case=schedule`
6. **Notification Sent**: Backend sends notification to WebSocket
7. **Popup Appears**: Frontend receives message and displays popup
8. **Sound Plays**: If enabled, plays notification sound (custom or default)
9. **Auto-Close**: Popup closes after 5 seconds

## Expected WebSocket Payloads

### Case 1: Basic Info

```json
{
  "notification": {
    "id": 123,
    "title": "Test Notification",
    "title_th": "ทดสอบการแจ้งเตือน",
    "message": "This is a basic test popup notification",
    "message_th": "นี่คือการแจ้งเตือนแบบป๊อปอัพทดสอบพื้นฐาน",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "test_case": "basic",
      "timestamp": 1737025800
    },
    "created_at": "2025-01-16T10:30:00Z"
  },
  "settings": {
    "enable_notification_sound": true,
    "notification_sound": "default"
  }
}
```

### Case 2: Schedule Reminder

```json
{
  "notification": {
    "title": "Upcoming Class",
    "title_th": "คาบเรียนใกล้ถึงแล้ว",
    "message": "Your class starts in 15 minutes",
    "message_th": "คาบเรียนของคุณจะเริ่มใน 15 นาที",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "action": "open_schedule",
      "schedule_id": 999,
      "session_id": 8888,
      "starts_at": "2025-01-16T10:45:00Z",
      "action_label": "View Schedule"
    }
  }
}
```

### Case 3: Warning

```json
{
  "notification": {
    "title": "Schedule Conflict",
    "title_th": "ตารางซ้อนกัน",
    "message": "You have two sessions overlapping. Please resolve.",
    "message_th": "คุณมี 2 คาบเรียนเวลาทับกัน กรุณาแก้ไข",
    "type": "warning",
    "channels": ["popup", "normal"],
    "data": {
      "action": "resolve_conflict",
      "conflicts": [
        {"session_id": 1001, "starts_at": "2025-01-17T10:00:00Z"},
        {"session_id": 1005, "starts_at": "2025-01-17T10:30:00Z"}
      ]
    }
  }
}
```

### Case 6: Normal Only (No Popup)

```json
{
  "notification": {
    "title": "Background Sync Complete",
    "message": "Data has been synchronized successfully.",
    "type": "success",
    "channels": ["normal"]
  }
}
```

### Case 11: Custom Sound

```json
{
  "notification": {
    "title": "Custom Sound Test",
    "message": "This notification should play your custom sound if enabled.",
    "type": "info",
    "channels": ["popup", "normal"]
  },
  "settings": {
    "enable_notification_sound": true,
    "notification_sound": "custom",
    "custom_sound_url": "https://ekls-test-bucket.s3.ap-southeast-1.amazonaws.com/custom-notification-sounds/123/2025/01/16/abc-def.mp3"
  }
}
```

### Case 13: Invitation

```json
{
  "notification": {
    "title": "Schedule Invitation",
    "title_th": "คำเชิญเข้าร่วมตาราง",
    "message": "You have been invited to join a schedule. Please confirm your participation.",
    "message_th": "คุณได้รับคำเชิญให้เข้าร่วมตาราง กรุณายืนยันการเข้าร่วม",
    "type": "info",
    "channels": ["popup", "normal"],
    "data": {
      "action": "respond_invitation",
      "schedule_id": 777,
      "invited_by": "Admin",
      "schedule_date": "2025-10-03",
      "requires_response": true,
      "action_label": "Respond to Invitation"
    }
  }
}
```

## Settings Integration

The notification system respects user settings:

### User Settings Fields

- `enable_notification_sound`: Boolean - Enable/disable notification sounds
- `notification_sound`: String - `"default"`, `"custom"`, or `"none"`
- `custom_sound_url`: String - S3 URL of custom uploaded sound file
- `custom_sound_filename`: String - Original filename of custom sound
- `custom_sound_s3_key`: String - S3 key for deletion/management

### Sound Playback Logic

```
if enable_notification_sound == true:
  if notification_sound == "custom" AND custom_sound_url exists:
    play(custom_sound_url)
  else if notification_sound == "default":
    play("/sounds/default-notification.mp3")
  else:
    # notification_sound == "none" - no sound
    pass
else:
  # Sounds disabled - no playback
  pass
```

## Troubleshooting

### No Popup Appears

1. Check WebSocket connection status
2. Verify case includes "popup" in channels (case 6 "normal_only" doesn't show popup)
3. Check browser console for JavaScript errors
4. Ensure notification handler is correctly implemented

### No Sound Plays

1. Verify `enable_notification_sound: true` in settings
2. Check `notification_sound` is "default" or "custom"
3. For custom sound, ensure `custom_sound_url` exists
4. Check browser permissions for audio playback
5. Test in different browsers (some block autoplay)

### WebSocket Not Connecting

1. Verify JWT token is valid
2. Check backend WebSocket endpoint is running
3. Ensure token is passed in URL: `?token=YOUR_JWT_TOKEN`
4. Check CORS and proxy settings in development

### Test Endpoint Returns Error

- **403 Forbidden**: JWT token missing or invalid
- **404 Not Found**: User ID doesn't exist
- **400 Bad Request**: Invalid test case name/number
- **500 Internal Server Error**: Backend service failure (check logs)

## Production Considerations

- Test endpoint is protected but accessible to all authenticated users
- Consider adding additional role restrictions for production
- Monitor usage to prevent abuse
- Rate limiting recommended for test endpoints
- Consider disabling in production or restricting to admin users only

## Related Documentation

- [API Documentation](../API_DOCUMENTATION.md)
- [WebSocket System](./WEBSOCKET.md)
- [Settings API](../SETTINGS.md)
- [Notification System](./LOGGING_SYSTEM.md)
