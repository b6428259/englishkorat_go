# 📊 Enhanced Logging System - English Korat API

## 🎯 ภาพรวมระบบ

ระบบ logging ใหม่ได้รับการออกแบบตาม **CIA (Confidentiality, Integrity, Availability)** principles พร้อมความสามารถดังนี้:

- ✅ **Redis Caching**: เก็บ logs ใน Redis 24 ชม. ก่อนส่งเข้า database
- ✅ **Auto-Archiving**: บีบอัดและย้ายไฟล์เก่าไป S3 หลัง 7 วัน
- ✅ **CIA Compliance**: ความปลอดภัยระดับ enterprise
- ✅ **Performance Optimized**: ไม่กระทบประสิทธิภาพการทำงาน
- ✅ **REST API**: จัดการและดู logs ผ่าน API endpoints

## 🏗️ สถาปัตยกรรม

```
HTTP Request → Enhanced Logging Middleware → Redis Cache (24h) → Database → S3 Archive (7d+)
                                                   ↓
                                            Log Management API
```

## 📁 ไฟล์ที่เกี่ยวข้อง

```
controllers/logs.go              # Log management controller
middleware/logging.go            # Enhanced logging middleware  
services/log_archive.go          # Archive and maintenance service
models/models.go                 # LogArchive model
routes/routes.go                 # Log API routes
```

## 🔐 CIA Security Features

### **Confidentiality (ความลับ)**
- ✅ Role-based access control (Owner/Admin เท่านั้น)
- ✅ Sensitive data masking ใน logs
- ✅ Encrypted storage ใน S3
- ✅ JWT authentication สำหรับทุก API calls

### **Integrity (ความถูกต้อง)**
- ✅ Integrity hash ในทุก log entry
- ✅ Tamper detection mechanism
- ✅ Immutable log archives
- ✅ Audit trail สำหรับ log operations

### **Availability (ความพร้อมใช้)**
- ✅ Redis failover (fallback เข้า database โดยตรง)
- ✅ Non-blocking logging (goroutines)
- ✅ Performance optimization
- ✅ Auto-recovery mechanisms

## 📊 Log Data Structure

### **Standard Log Entry**
```json
{
  "id": 123,
  "user_id": 1,
  "action": "CREATE",
  "resource": "students",
  "resource_id": 45,
  "ip_address": "192.168.1.100",
  "user_agent": "Mozilla/5.0...",
  "created_at": "2025-09-08T15:30:00Z",
  "details": {
    "original_details": {...},
    "integrity_hash": "a1b2c3d4e5f6...",
    "session_id": "sess_123456",
    "request_id": "req_789012",
    "forwarded_for": "10.0.0.1",
    "real_ip": "203.154.1.100",
    "protocol": "https",
    "method": "POST",
    "path": "/api/students",
    "query": "?branch_id=1",
    "status_code": 201,
    "content_length": 1024,
    "referer": "https://app.englishkorat.com",
    "timestamp_utc": 1694181800,
    "timezone": "Asia/Bangkok"
  }
}
```

## 🚀 API Endpoints

### **📋 ดู Logs**
```http
GET /api/logs
Authorization: Bearer {jwt_token}
```

**Query Parameters:**
- `page` - หน้าที่ต้องการ (default: 1)
- `limit` - จำนวนต่อหน้า (default: 50, max: 100)
- `user_id` - Filter ตาม user ID
- `action` - Filter ตาม action (CREATE, UPDATE, DELETE)
- `resource` - Filter ตาม resource (users, students, etc.)
- `ip_address` - Filter ตาม IP
- `start_date` - วันที่เริ่มต้น (YYYY-MM-DD)
- `end_date` - วันที่สิ้นสุด (YYYY-MM-DD)

**Response:**
```json
{
  "logs": [...],
  "total": 1500,
  "page": 1,
  "limit": 50,
  "total_pages": 30
}
```

### **📈 สถิติ Logs**
```http
GET /api/logs/stats
Authorization: Bearer {jwt_token}
```

**Response:**
```json
{
  "total": 15420,
  "total_today": 245,
  "total_this_week": 1680,
  "total_this_month": 6789,
  "action_breakdown": {
    "CREATE": 5200,
    "UPDATE": 7800,
    "DELETE": 2420
  },
  "resource_breakdown": {
    "users": 3400,
    "students": 8900,
    "teachers": 2100,
    "courses": 1020
  },
  "hourly_activity": {
    "00:00": 12, "01:00": 8, "02:00": 4, ..., "23:00": 25
  },
  "top_users": [
    {"user_id": 1, "username": "admin", "role": "admin", "count": 234},
    ...
  ],
  "recent_activity": [...]
}
```

### **🔍 ดู Log เดียว**
```http
GET /api/logs/{id}
Authorization: Bearer {jwt_token}
```

### **📤 Export Logs**
```http
GET /api/logs/export?start_date=2025-09-01&end_date=2025-09-08
Authorization: Bearer {jwt_token}
```
**Response:** CSV file download

### **🗂️ Cache Management**
```http
POST /api/logs/flush-cache
Authorization: Bearer {jwt_token}
```

### **🗑️ ลบ Logs เก่า**
```http
DELETE /api/logs/old?days=30
Authorization: Bearer {jwt_token}
```

## ⚙️ การตั้งค่า Environment

### **Redis Configuration**
```env
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=yourpassword
```

### **S3 Configuration**
```env
AWS_REGION=ap-southeast-1
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
S3_BUCKET_NAME=your-bucket-name
```

### **Log Settings**
```env
LOG_ARCHIVE_DAYS=7d           # วันที่จะ archive (default: 7)
LOG_CACHE_TTL=24h             # ระยะเวลา cache (default: 24h)
LOG_LEVEL=info                # ระดับ log
LOG_FILE=logs/app.log         # ไฟล์ log
```

## 🔄 Auto-Maintenance Schedule

### **Cache Flush (ทุกชั่วโมง)**
- ย้าย logs จาก Redis เข้า database หลัง 24 ชม.
- ลบ cache entries ที่หมดอายุ
- Error handling และ fallback

### **Archive Process (ทุกวันเวลา 02:00)**
- รวบรวม logs เก่ากว่า 7 วัน
- สร้างไฟล์ ZIP พร้อม metadata
- อัปโหลดไป S3 bucket
- ลบ logs เก่าจาก database
- สร้าง archive metadata record

## 📦 Archive Structure

### **ZIP File Contents:**
```
activity_logs_2025-09-01.zip
├── activity_logs.json        # Main log data (JSON format)
├── activity_logs.csv         # CSV format for easy viewing
└── metadata.json            # Archive metadata
```

### **S3 Bucket Structure:**
```
logs/
└── archived/
    └── 2025/
        └── 09/
            ├── activity_logs_2025-09-01.zip
            ├── activity_logs_2025-09-08.zip
            └── ...
```

## 🚨 Error Handling

### **Redis Unavailable**
```go
// Auto-fallback เข้า database โดยตรง
if redisClient == nil {
    database.DB.Create(&activityLog)
}
```

### **S3 Upload Failed**
- Retry mechanism (3 attempts)
- Local backup ใน `/tmp/failed_archives/`
- Admin notification via logs
- Manual recovery procedures

### **Database Connection Lost**
- Queue logs ใน Redis
- Extended TTL (48 hours)
- Auto-retry เมื่อ connection กลับมา

## 🎯 Performance Metrics

### **Baseline Performance:**
- **Logging Overhead**: < 2ms per request
- **Memory Usage**: < 50MB for Redis cache
- **Storage**: ~1GB/month for 100K requests/day
- **Archive Size**: ~10MB per week (compressed)

### **Monitoring Metrics:**
```json
{
  "redis_cache_size": "45.2MB",
  "cache_hit_ratio": "94.5%",
  "avg_flush_time": "1.2s",
  "archive_success_rate": "99.8%",
  "storage_cost_monthly": "$2.40"
}
```

## 🔧 การ Troubleshooting

### **ปัญหาที่พบบ่อย:**

1. **Logs ไม่เข้า Database**
   ```bash
   # ตรวจสอบ Redis connection
   curl localhost:8081  # Redis Commander UI
   
   # Manual flush
   POST /api/logs/flush-cache
   ```

2. **Archive ล้มเหลว**
   ```bash
   # ตรวจสอบ S3 permissions
   aws s3 ls s3://your-bucket/logs/
   
   # Manual archive
   curl -X POST /api/admin/archive-logs
   ```

3. **Performance Issues**
   ```bash
   # ดู Redis memory usage
   redis-cli info memory
   
   # ปรับ cache size
   redis-cli config set maxmemory 256mb
   ```

## 📊 Dashboard Integration

### **Grafana Queries:**
```sql
-- Total logs per hour
SELECT date_trunc('hour', created_at) as time, count(*) 
FROM activity_logs 
WHERE created_at > now() - interval '24 hours'
GROUP BY time;

-- Top users by activity
SELECT users.username, count(*) as activity_count
FROM activity_logs 
JOIN users ON activity_logs.user_id = users.id
WHERE created_at > now() - interval '7 days'
GROUP BY users.username
ORDER BY activity_count DESC;
```

### **Alerts:**
- Redis memory > 80%
- Archive failure
- Log volume spike (>1000% normal)
- Suspicious activity patterns

## 🎉 การใช้งาน

1. **Setup**: ระบบจะทำงานอัตโนมัติหลัง server start
2. **Monitor**: ใช้ `/api/logs/stats` ดูสถิติ
3. **Export**: ใช้ `/api/logs/export` สำหรับ compliance
4. **Archive**: ตรวจสอบ S3 bucket สำหรับไฟล์เก่า

---

**🔒 Security Note:** ระบบนี้ออกแบบสำหรับ production environment พร้อม enterprise-grade security และ compliance requirements.
