package services

import (
	"context"
	"englishkorat_go/config"
	"englishkorat_go/database"
	"fmt"
	"runtime"
	"strings"
	"time"
)

const (
	overallStatusOK       = "ok"
	overallStatusDegraded = "degraded"
	overallStatusCritical = "critical"

	dependencyStatusUp       = "up"
	dependencyStatusDown     = "down"
	dependencyStatusDisabled = "disabled"

	defaultServiceName = "English Korat API"
	defaultVersion     = "1.0.0"
	defaultTimeout     = 1500 * time.Millisecond
)

// HealthService aggregates application health information for reporting endpoints.
type HealthService struct {
	serviceName string
	version     string
	startTime   time.Time
	timeout     time.Duration
}

// HealthReport represents the JSON response for health endpoints.
type HealthReport struct {
	Status        string             `json:"status"`
	Service       string             `json:"service"`
	Version       string             `json:"version"`
	Environment   string             `json:"environment"`
	Time          time.Time          `json:"time"`
	UptimeSeconds float64            `json:"uptime_seconds"`
	UptimeHuman   string             `json:"uptime_human"`
	Dependencies  []DependencyStatus `json:"dependencies"`
	Metrics       HealthMetrics      `json:"metrics"`
	Flags         HealthFlags        `json:"flags"`
	System        HealthSystem       `json:"system"`
}

// DependencyStatus captures the health of a single external dependency.
type DependencyStatus struct {
	Name      string                 `json:"name"`
	Status    string                 `json:"status"`
	LatencyMs int64                  `json:"latency_ms"`
	Error     string                 `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthMetrics captures runtime metrics for diagnostics.
type HealthMetrics struct {
	Goroutines int            `json:"goroutines"`
	Memory     MemoryMetrics  `json:"memory"`
	Database   *DatabaseStats `json:"database,omitempty"`
}

// MemoryMetrics captures Go memory statistics.
type MemoryMetrics struct {
	AllocBytes      uint64 `json:"alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
	HeapAllocBytes  uint64 `json:"heap_alloc_bytes"`
	HeapObjects     uint64 `json:"heap_objects"`
	LastGCUnix      *int64 `json:"last_gc_unix,omitempty"`
	PauseTotalNs    uint64 `json:"pause_total_ns"`
}

// DatabaseStats captures statistics from the SQL connection pool.
type DatabaseStats struct {
	OpenConnections    int   `json:"open_connections"`
	InUse              int   `json:"in_use"`
	Idle               int   `json:"idle"`
	WaitCount          int64 `json:"wait_count"`
	WaitDurationMs     int64 `json:"wait_duration_ms"`
	MaxOpenConnections int   `json:"max_open_connections"`
}

// HealthFlags exposes feature toggles that influence runtime behaviour.
type HealthFlags struct {
	SkipMigrate           bool `json:"skip_migrate"`
	PruneColumns          bool `json:"prune_columns"`
	UseRedisNotifications bool `json:"use_redis_notifications"`
}

// HealthSystem exposes static information about the running system.
type HealthSystem struct {
	GoVersion string `json:"go_version"`
	GoOS      string `json:"go_os"`
	GoArch    string `json:"go_arch"`
}

// NewHealthService creates a new HealthService with sensible defaults.
func NewHealthService(serviceName, version string) *HealthService {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = defaultServiceName
	}
	if strings.TrimSpace(version) == "" {
		version = defaultVersion
	}

	return &HealthService{
		serviceName: serviceName,
		version:     version,
		startTime:   time.Now(),
		timeout:     defaultTimeout,
	}
}

// SetStartTime overrides the start time used for uptime calculations.
func (s *HealthService) SetStartTime(t time.Time) {
	if !t.IsZero() {
		s.startTime = t
	}
}

// SetTimeout overrides the timeout used when probing dependencies.
func (s *HealthService) SetTimeout(d time.Duration) {
	if d > 0 {
		s.timeout = d
	}
}

// GetHealthReport collects the current health information.
func (s *HealthService) GetHealthReport() HealthReport {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	report := HealthReport{
		Status:      overallStatusOK,
		Service:     s.serviceName,
		Version:     s.version,
		Environment: currentEnvironment(),
		Time:        time.Now().UTC(),
	}

	uptime := time.Since(s.startTime)
	if uptime < 0 {
		uptime = 0
	}
	report.UptimeSeconds = uptime.Seconds()
	report.UptimeHuman = humanizeDuration(uptime)

	var deps []DependencyStatus

	dbDep, dbMetrics, dbStatus := s.checkDatabase(ctx)
	if dbDep != nil {
		deps = append(deps, *dbDep)
	}
	report.Status = combineStatus(report.Status, dbStatus)

	redisDep, redisStatus := s.checkRedis(ctx)
	if redisDep != nil {
		deps = append(deps, *redisDep)
	}
	report.Status = combineStatus(report.Status, redisStatus)

	report.Dependencies = deps
	report.Metrics = collectSystemMetrics(dbMetrics)
	report.Flags = collectFlags()
	report.System = HealthSystem{
		GoVersion: runtime.Version(),
		GoOS:      runtime.GOOS,
		GoArch:    runtime.GOARCH,
	}

	return report
}

// HTTPStatusForOverall maps a health status to an HTTP status code.
func (s *HealthService) HTTPStatusForOverall(status string) int {
	switch status {
	case overallStatusCritical:
		return 503
	default:
		return 200
	}
}

func (s *HealthService) checkDatabase(ctx context.Context) (*DependencyStatus, *DatabaseStats, string) {
	dep := &DependencyStatus{Name: "mysql"}
	overall := overallStatusOK

	if database.DB == nil {
		dep.Status = dependencyStatusDown
		dep.Error = "database connection not initialised"
		overall = overallStatusCritical
		return dep, nil, overall
	}

	sqlDB, err := database.DB.DB()
	if err != nil {
		dep.Status = dependencyStatusDown
		dep.Error = fmt.Sprintf("sql DB handle error: %v", err)
		overall = overallStatusCritical
		return dep, nil, overall
	}

	pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	start := time.Now()
	err = sqlDB.PingContext(pingCtx)
	cancel()
	dep.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		dep.Status = dependencyStatusDown
		dep.Error = err.Error()
		overall = overallStatusCritical
		return dep, nil, overall
	}

	dep.Status = dependencyStatusUp
	stats := sqlDB.Stats()
	dep.Details = map[string]interface{}{
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration_ms":     stats.WaitDuration.Milliseconds(),
		"max_open_connections": stats.MaxOpenConnections,
	}

	dbMetrics := &DatabaseStats{
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDurationMs:     stats.WaitDuration.Milliseconds(),
		MaxOpenConnections: stats.MaxOpenConnections,
	}

	return dep, dbMetrics, overall
}

func (s *HealthService) checkRedis(ctx context.Context) (*DependencyStatus, string) {
	dep := &DependencyStatus{Name: "redis"}
	overall := overallStatusOK

	client := database.GetRedisClient()
	useRedis := config.AppConfig != nil && config.AppConfig.UseRedisNotifications

	if client == nil {
		if useRedis {
			dep.Status = dependencyStatusDown
			dep.Error = "redis client not initialised"
			overall = overallStatusDegraded
		} else {
			dep.Status = dependencyStatusDisabled
		}
		return dep, overall
	}

	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	start := time.Now()
	res := client.Ping(pingCtx)
	cancel()
	dep.LatencyMs = time.Since(start).Milliseconds()

	if err := res.Err(); err != nil {
		dep.Status = dependencyStatusDown
		dep.Error = err.Error()
		if useRedis {
			overall = overallStatusDegraded
		}
		return dep, overall
	}

	dep.Status = dependencyStatusUp
	addr := client.Options().Addr
	if useRedis {
		dep.Details = map[string]interface{}{
			"address": addr,
			"mode":    "notifications",
		}
	} else {
		dep.Details = map[string]interface{}{
			"address": addr,
			"mode":    "optional",
		}
	}

	return dep, overall
}

func collectSystemMetrics(dbMetrics *DatabaseStats) HealthMetrics {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	metrics := HealthMetrics{
		Goroutines: runtime.NumGoroutine(),
		Memory: MemoryMetrics{
			AllocBytes:      mem.Alloc,
			TotalAllocBytes: mem.TotalAlloc,
			SysBytes:        mem.Sys,
			HeapAllocBytes:  mem.HeapAlloc,
			HeapObjects:     mem.HeapObjects,
			PauseTotalNs:    mem.PauseTotalNs,
		},
		Database: dbMetrics,
	}

	if mem.LastGC != 0 {
		last := time.Unix(0, int64(mem.LastGC))
		unix := last.Unix()
		metrics.Memory.LastGCUnix = &unix
	}

	return metrics
}

func collectFlags() HealthFlags {
	if config.AppConfig == nil {
		return HealthFlags{}
	}

	return HealthFlags{
		SkipMigrate:           config.AppConfig.SkipMigrate,
		PruneColumns:          config.AppConfig.PruneColumns,
		UseRedisNotifications: config.AppConfig.UseRedisNotifications,
	}
}

func currentEnvironment() string {
	if config.AppConfig == nil {
		return "unknown"
	}
	env := strings.TrimSpace(config.AppConfig.AppEnv)
	if env == "" {
		return "unknown"
	}
	return env
}

func combineStatus(current, candidate string) string {
	order := map[string]int{
		overallStatusOK:       0,
		overallStatusDegraded: 1,
		overallStatusCritical: 2,
	}

	if _, ok := order[current]; !ok {
		current = overallStatusOK
	}

	if v, ok := order[candidate]; ok && v > order[current] {
		return candidate
	}
	return current
}

func humanizeDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}

	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d %= 24 * time.Hour
	hours := d / time.Hour
	d %= time.Hour
	minutes := d / time.Minute
	d %= time.Minute
	seconds := d / time.Second

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	return strings.Join(parts, " ")
}
