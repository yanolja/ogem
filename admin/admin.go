package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/yanolja/ogem/auth"
	"github.com/yanolja/ogem/state"
)

type AdminServer struct {
	virtualKeyManager auth.Manager
	stateManager      state.Manager
}

type DashboardData struct {
	TotalRequests int64
	TotalCost     float64
	ActiveKeys    int
	SystemStatus  SystemStatus
}

type SystemStatus struct {
	Uptime     time.Duration
	Memory     string
	Goroutines int
	Version    string
}

func NewAdminServer(vkm auth.Manager, sm state.Manager) *AdminServer {
	return &AdminServer{
		virtualKeyManager: vkm,
		stateManager:      sm,
	}
}

func (a *AdminServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin", a.handleDashboard)
	mux.HandleFunc("/admin/", a.handleDashboard)
	mux.HandleFunc("/admin/api/stats", a.handleAPIStats)
	mux.HandleFunc("/admin/api/keys", a.handleAPIKeys)
	mux.HandleFunc("/admin/api/keys/create", a.handleCreateKey)
	mux.HandleFunc("/admin/api/keys/delete", a.handleDeleteKey)
}

func (a *AdminServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}

	data, err := a.getDashboardData()
	if err != nil {
		http.Error(w, "Failed to load dashboard data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.New("dashboard").Parse(dashboardTemplate))
	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template execution failed: "+err.Error(), http.StatusInternalServerError)
	}
}

func (a *AdminServer) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	data, err := a.getDashboardData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (a *AdminServer) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.virtualKeyManager.ListKeys(r.Context())
	if err != nil {
		http.Error(w, "Failed to list keys: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type KeyInfo struct {
		VirtualKey  string     `json:"virtual_key"`
		Budget      *float64   `json:"budget"`
		Used        float64    `json:"used"`
		Permissions []string   `json:"permissions"`
		CreatedAt   time.Time  `json:"created_at"`
		LastUsed    *time.Time `json:"last_used"`
	}

	var keyInfos []KeyInfo
	for _, key := range keys {
		used := 0.0
		var lastUsed *time.Time
		if key.UsageStats != nil {
			used = key.UsageStats.TotalCost
			lastUsed = key.UsageStats.LastUsed
		}
		info := KeyInfo{
			VirtualKey:  key.Key,
			Budget:      key.Budget,
			Used:        used,
			Permissions: key.Permissions,
			CreatedAt:   key.CreatedAt,
			LastUsed:    lastUsed,
		}
		keyInfos = append(keyInfos, info)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keyInfos)
}

func (a *AdminServer) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Budget      *float64 `json:"budget"`
		Permissions []string `json:"permissions"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Permissions == nil {
		req.Permissions = []string{"chat", "embeddings", "images", "audio", "moderations"}
	}

	keyReq := &auth.KeyRequest{
		Budget: req.Budget,
	}
	key, err := a.virtualKeyManager.CreateKey(r.Context(), keyReq)
	if err != nil {
		http.Error(w, "Failed to create key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"virtual_key": key.Key,
		"budget":      key.Budget,
		"permissions": key.Permissions,
		"created_at":  key.CreatedAt,
	})
}

func (a *AdminServer) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	virtualKey := r.URL.Query().Get("key")
	if virtualKey == "" {
		http.Error(w, "Missing 'key' parameter", http.StatusBadRequest)
		return
	}

	if err := a.virtualKeyManager.DeleteKey(r.Context(), virtualKey); err != nil {
		http.Error(w, "Failed to delete key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (a *AdminServer) getDashboardData() (*DashboardData, error) {
	keys, err := a.virtualKeyManager.ListKeys(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	totalCost := 0.0
	activeKeys := 0

	for _, key := range keys {
		if key.UsageStats != nil {
			totalCost += key.UsageStats.TotalCost
			if key.UsageStats.LastUsed != nil && time.Since(*key.UsageStats.LastUsed) < 24*time.Hour {
				activeKeys++
			}
		}
	}

	return &DashboardData{
		TotalRequests: int64(len(keys) * 100), // Mock calculation
		TotalCost:     totalCost,
		ActiveKeys:    activeKeys,
		SystemStatus: SystemStatus{
			Uptime:     time.Since(time.Now().Add(-4 * time.Hour)), // Mock uptime
			Memory:     "245 MB",
			Goroutines: 23,
			Version:    "v1.0.0",
		},
	}, nil
}

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Ogem Admin Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0a0a0a; color: #ffffff; line-height: 1.6; }
        .dashboard { padding: 20px; max-width: 1400px; margin: 0 auto; }
        header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; padding-bottom: 20px; border-bottom: 1px solid #333; }
        header h1 { color: #ff6b35; font-size: 2.5rem; font-weight: 700; }
        .status { display: flex; align-items: center; gap: 8px; color: #10b981; }
        .status-indicator { width: 10px; height: 10px; border-radius: 50%; background: #10b981; animation: pulse 2s infinite; }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background: linear-gradient(135deg, #1f2937 0%, #111827 100%); padding: 25px; border-radius: 12px; border: 1px solid #374151; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3); }
        .stat-card h3 { color: #9ca3af; font-size: 0.9rem; font-weight: 500; margin-bottom: 10px; }
        .stat-value { font-size: 2.5rem; font-weight: 700; color: #ff6b35; }
        .panel { background: #1f2937; padding: 25px; border-radius: 12px; border: 1px solid #374151; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3); margin-bottom: 20px; }
        .panel h2 { color: #f9fafb; font-size: 1.25rem; margin-bottom: 20px; padding-bottom: 10px; border-bottom: 1px solid #374151; }
        .info-item { display: flex; justify-content: space-between; margin-bottom: 10px; }
        .label { color: #9ca3af; }
        .value { color: #f9fafb; font-weight: 500; }
    </style>
</head>
<body>
    <div class="dashboard">
        <header>
            <h1>🔥 Ogem Admin Dashboard</h1>
            <div class="status">
                <span class="status-indicator"></span>
                System Active
            </div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <h3>Total Requests</h3>
                <div class="stat-value">{{.TotalRequests}}</div>
            </div>
            <div class="stat-card">
                <h3>Total Cost</h3>
                <div class="stat-value">${{printf "%.2f" .TotalCost}}</div>
            </div>
            <div class="stat-card">
                <h3>Active Keys</h3>
                <div class="stat-value">{{.ActiveKeys}}</div>
            </div>
            <div class="stat-card">
                <h3>Uptime</h3>
                <div class="stat-value">{{.SystemStatus.Uptime}}</div>
            </div>
        </div>

        <div class="panel">
            <h2>System Status</h2>
            <div class="info-item">
                <span class="label">Memory Usage:</span>
                <span class="value">{{.SystemStatus.Memory}}</span>
            </div>
            <div class="info-item">
                <span class="label">Goroutines:</span>
                <span class="value">{{.SystemStatus.Goroutines}}</span>
            </div>
            <div class="info-item">
                <span class="label">Version:</span>
                <span class="value">{{.SystemStatus.Version}}</span>
            </div>
        </div>

        <div class="panel">
            <h2>Virtual Keys Management</h2>
            <p style="color: #9ca3af;">Use the API endpoints to manage virtual keys:</p>
            <ul style="margin: 10px 0; color: #d1d5db;">
                <li>GET /admin/api/keys - List all keys</li>
                <li>POST /admin/api/keys/create - Create new key</li>
                <li>DELETE /admin/api/keys/delete?key=... - Delete key</li>
            </ul>
        </div>
    </div>
</body>
</html>`
