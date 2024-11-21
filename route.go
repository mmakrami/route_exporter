package main

import (
    "context"
    "log"
    "net/http"
    "os/exec"
    "strings"
    "sync"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    // متریکی برای نشان دادن تغییر در روت‌های IP (1 برای تغییر، 0 برای عدم تغییر)
    ipRouteChanges = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "ip_route_changes",
            Help: "Indicates if there was a change in IP routes (1 for change, 0 for no change)",
        },
    )

    // متریکی برای لیست روت‌های فعلی IP
    ipRoutesList = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ip_routes_list",
            Help: "Current IP routes",
        },
        []string{"route"},
    )

    // متریکی برای ثبت زمان آخرین تغییر هر روت IP
    ipRouteLastChangeTimestamp = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ip_route_last_change_timestamp",
            Help: "Timestamp of the last change for each IP route",
        },
        []string{"route"},
    )

    previousRoutes = make(map[string]struct{})
    mu             sync.Mutex
    collectInterval = 30 * time.Second // می‌توانید این زمان را بر اساس نیاز خود تنظیم کنید
)

func init() {
    prometheus.MustRegister(ipRouteChanges)
    prometheus.MustRegister(ipRoutesList)
    prometheus.MustRegister(ipRouteLastChangeTimestamp)
}

func collectRoutes(ctx context.Context) {
    cmd := exec.CommandContext(ctx, "ip", "route")
    output, err := cmd.Output()
    if err != nil {
        log.Printf("Error executing command: %v", err)
        return
    }

    lines := strings.Split(string(output), "\n")
    currentRoutes := make(map[string]struct{})

    changesDetected := false

    for _, line := range lines {
        if line == "" {
            continue
        }

        route := strings.TrimSpace(line)
        currentRoutes[route] = struct{}{}

        // ثبت روت در متریک‌ها
        ipRoutesList.WithLabelValues(route).Set(1)
        ipRouteLastChangeTimestamp.WithLabelValues(route).Set(float64(time.Now().Unix()))

        mu.Lock()
        if _, exists := previousRoutes[route]; !exists {
            // روت جدید اضافه شده است
            log.Printf("Route added: %s", route)
            changesDetected = true
        }
        mu.Unlock()
    }

    mu.Lock()
    for route := range previousRoutes {
        if _, exists := currentRoutes[route]; !exists {
            // روت حذف شده است
            log.Printf("Route removed: %s", route)
            changesDetected = true

            // حذف روت از متریک‌ها
            ipRoutesList.WithLabelValues(route).Set(0)
            ipRouteLastChangeTimestamp.WithLabelValues(route).Set(float64(time.Now().Unix()))
        }
    }
    previousRoutes = currentRoutes
    mu.Unlock()

    if changesDetected {
        ipRouteChanges.Set(1)
    } else {
        ipRouteChanges.Set(0)
    }
}

func main() {
    http.Handle("/metrics", promhttp.Handler())
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go func() {
        for {
            collectRoutes(ctx)
            time.Sleep(collectInterval)
        }
    }()

    log.Println("Starting server on :8081")
    if err := http.ListenAndServe(":8081", nil); err != nil {
        log.Fatalf("Error starting server: %v", err)
    }
}
