# 行程地址显示 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为行程开始和行程结束通知增加中文地址显示，使用高德地图逆地理编码 API。

**Architecture:** 新建 geocode 包封装高德 API 调用和坐标转换；修改 db.go 获取坐标数据；修改 handler.go 调用地址 API 并调整通知格式。

**Tech Stack:** Go, 高德 Web 服务 API, HTTP client

**Spec:** `docs/superpowers/specs/2026-06-16-trip-address-design.md`

---

### Task 1: config.go 新增 AmapAPIKey 字段

**Files:**
- Modify: `internal/config/config.go:10-24` (Config 结构体)
- Modify: `internal/config/config.go:26-44` (Load 函数)

- [ ] **Step 1: 在 Config 结构体中新增字段**

在 `PushDebounceSec int` 字段之后添加：

```go
	AmapAPIKey string // 高德地图 API Key
```

完整结构体变为：

```go
type Config struct {
	APIToken        string
	DBHost          string
	DBUser          string
	DBPass          string
	DBName          string
	DBPort          int
	MQTTHost        string
	MQTTPort        int
	MQTTUser        string
	MQTTPass        string
	CarID           int
	LogLevel        string
	PushDebounceSec int // 推送防抖初始时间，后续会进行3次指数退避重试，按(次数-1)倍增加
	AmapAPIKey      string // 高德地图 API Key
}
```

- [ ] **Step 2: 在 Load 函数中读取环境变量**

在 `PushDebounceSec: mustInt(os.Getenv("PUSH_DEBOUNCE_SECONDS"), 5),` 之后添加：

```go
		AmapAPIKey:      os.Getenv("AMAP_API_KEY"),
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: config 新增 AmapAPIKey 高德地图 API Key 配置"
```

---

### Task 2: 创建 geocode 包 - 坐标转换

**Files:**
- Create: `internal/geocode/convert.go`

- [ ] **Step 1: 创建 geocode 目录**

```bash
mkdir -p internal/geocode
```

- [ ] **Step 2: 创建 convert.go 文件，实现 WGS84 → GCJ02 转换**

```go
package geocode

import "math"

// WGS84 转 GCJ02 坐标转换常量
const (
	a  = 6378245.0
	ee = 0.00669342162296594323
)

// WGS84ToGCJ02 将 WGS84 坐标转换为 GCJ02（火星坐标系）
func WGS84ToGCJ02(lat, lng float64) (float64, float64) {
	if outOfChina(lat, lng) {
		return lat, lng
	}
	dLat := transformLat(lng - 105.0, lat - 35.0)
	dLng := transformLng(lng - 105.0, lat - 35.0)
	radLat := lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - ee * magic * magic
	sqrtMagic := math.Sqrt(magic)
	dLat = (dLat * 180.0) / ((a * (1 - ee)) / (magic * sqrtMagic) * math.Pi)
	dLng = (dLng * 180.0) / (a / sqrtMagic * math.Cos(radLat) * math.Pi)
	return lat + dLat, lng + dLng
}

func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0 * x + 3.0 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * math.Sqrt(math.Abs(x))
	ret += (20.0 * math.Sin(6.0 * x * math.Pi) + 20.0 * math.Sin(2.0 * x * math.Pi)) * 2.0 / 3.0
	ret += (20.0 * math.Sin(y * math.Pi) + 40.0 * math.Sin(y / 3.0 * math.Pi)) * 2.0 / 3.0
	ret += (160.0 * math.Sin(y / 12.0 * math.Pi) + 320 * math.Sin(y * math.Pi / 30.0)) * 2.0 / 3.0
	return ret
}

func transformLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * math.Sqrt(math.Abs(x))
	ret += (20.0 * math.Sin(6.0 * x * math.Pi) + 20.0 * math.Sin(2.0 * x * math.Pi)) * 2.0 / 3.0
	ret += (20.0 * math.Sin(x * math.Pi) + 40.0 * math.Sin(x / 3.0 * math.Pi)) * 2.0 / 3.0
	ret += (150.0 * math.Sin(x / 12.0 * math.Pi) + 300.0 * math.Sin(x / 30.0 * math.Pi)) * 2.0 / 3.0
	return ret
}

func outOfChina(lat, lng float64) bool {
	return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 4: Commit**

```bash
git add internal/geocode/convert.go
git commit -m "feat: 新增 WGS84 到 GCJ02 坐标转换算法"
```

---

### Task 3: 创建 geocode 包 - 高德 API 调用

**Files:**
- Create: `internal/geocode/amap.go`

- [ ] **Step 1: 创建 amap.go 文件**

```go
package geocode

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AmapClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewAmapClient(apiKey string) *AmapClient {
	return &AmapClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetAddress 获取简短地址（如"萧山万象汇"）
// 输入 WGS84 坐标，内部自动转换为 GCJ02
func (c *AmapClient) GetAddress(lat, lng float64) string {
	if c.apiKey == "" {
		return "定位获取失败"
	}

	// WGS84 → GCJ02 转换
	gcjLat, gcjLng := WGS84ToGCJ02(lat, lng)

	resp, err := c.reverseGeocode(gcjLat, gcjLng)
	if err != nil {
		log.Printf("[高德API] 调用失败: %v", err)
		return "定位获取失败"
	}

	return formatAddress(resp)
}

// reverseGeocode 调用高德逆地理编码 API
func (c *AmapClient) reverseGeocode(lat, lng float64) (*RegeoResponse, error) {
	location := fmt.Sprintf("%.6f,%.6f", lng, lat)
	apiURL := fmt.Sprintf(
		"https://restapi.amap.com/v3/geocode/regeo?key=%s&location=%s&extensions=all",
		url.QueryEscape(c.apiKey),
		url.QueryEscape(location),
	)

	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result RegeoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Status != "1" {
		return nil, fmt.Errorf("高德API返回状态码: %s", result.Status)
	}

	return &result, nil
}

// formatAddress 格式化地址：区 + POI名称（去掉「区」字）
func formatAddress(resp *RegeoResponse) string {
	comp := resp.Regeocode.AddressComponent

	// 获取区名，去掉「区」字
	district := strings.TrimSuffix(comp.District, "区")

	// 优先使用 POI 名称
	if len(resp.Regeocode.Pois) > 0 && resp.Regeocode.Pois[0].Name != "" {
		return district + resp.Regeocode.Pois[0].Name
	}

	// 无 POI 则使用街道名
	if comp.Street != "" && comp.StreetNumber != "" {
		return district + comp.Street + comp.StreetNumber
	}
	if comp.Street != "" {
		return district + comp.Street
	}

	// 都没有则返回区名
	if district != "" {
		return district
	}

	return "定位获取失败"
}

// RegeoResponse 高德逆地理编码响应结构
type RegeoResponse struct {
	Status    string `json:"status"`
	Regeocode struct {
		AddressComponent struct {
			District      string `json:"district"`
			Street        string `json:"street"`
			StreetNumber  string `json:"streetNumber"`
		} `json:"addressComponent"`
		Pois []struct {
			Name string `json:"name"`
		} `json:"pois"`
	} `json:"regeocode"`
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add internal/geocode/amap.go
git commit -m "feat: 新增高德地图逆地理编码 API 调用封装"
```

---

### Task 4: db.go 新增 GetLatestPosition 和坐标字段

**Files:**
- Modify: `internal/db/db.go:63-83` (DriveWithSOC 结构体和 GetLatestDrive 函数)
- Modify: `internal/db/db.go` (新增 GetLatestPosition 函数)

- [ ] **Step 1: 修改 DriveWithSOC 结构体，新增坐标字段**

将 DriveWithSOC 结构体改为：

```go
type DriveWithSOC struct {
	models.Drive
	StartSOC    float64 `gorm:"column:start_soc"`
	EndSOC      float64 `gorm:"column:end_soc"`
	StartLat    float64 `gorm:"column:start_lat"`
	StartLng    float64 `gorm:"column:start_lng"`
	EndLat      float64 `gorm:"column:end_lat"`
	EndLng      float64 `gorm:"column:end_lng"`
}
```

- [ ] **Step 2: 修改 GetLatestDrive SQL，新增坐标查询**

将 GetLatestDrive 函数中的 SQL 改为：

```go
	err := DB.Table("drives d").
		Select(`d.*, 
				COALESCE(start_pos.usable_battery_level, start_pos.battery_level, 0) as start_soc,
				COALESCE(end_pos.usable_battery_level, end_pos.battery_level, 0) as end_soc,
				start_pos.latitude as start_lat,
				start_pos.longitude as start_lng,
				end_pos.latitude as end_lat,
				end_pos.longitude as end_lng`).
		Joins("LEFT JOIN positions start_pos ON d.start_position_id = start_pos.id").
		Joins("LEFT JOIN positions end_pos ON d.end_position_id = end_pos.id").
		Where("d.car_id = ?", carID).
		Order("d.id desc").
		First(&result).Error
```

- [ ] **Step 3: 新增 GetLatestPosition 函数**

在 `GetLatestDrive` 函数之后添加：

```go
// Position 最新位置信息
type Position struct {
	Latitude  float64
	Longitude float64
}

// GetLatestPosition 获取车辆最新位置坐标
func GetLatestPosition(carID int) (*Position, error) {
	var result Position

	err := DB.Table("positions").
		Select("latitude, longitude").
		Where("car_id = ?", carID).
		Order("id desc").
		First(&result).Error

	return &result, err
}
```

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 5: Commit**

```bash
git add internal/db/db.go
git commit -m "feat: db 新增坐标字段查询和 GetLatestPosition 函数"
```

---

### Task 5: handler.go 修改行程开始通知

**Files:**
- Modify: `internal/mqtt/handler.go:252-280` (processTripStart 函数)

- [ ] **Step 1: 在 handler.go 顶部新增 import**

在 import 块中添加 `"github.com/wen-ryon/tete-manager-notifier/internal/geocode"`：

```go
import (
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wen-ryon/tete-manager-notifier/internal/config"
	"github.com/wen-ryon/tete-manager-notifier/internal/db"
	"github.com/wen-ryon/tete-manager-notifier/internal/geocode"
	"github.com/wen-ryon/tete-manager-notifier/internal/models"
	"github.com/wen-ryon/tete-manager-notifier/internal/notifier"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)
```

- [ ] **Step 2: 修改 processTripStart 函数**

将 processTripStart 函数改为：

```go
func (c *Client) processTripStart() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lastState != "driving" || c.tripStartNotified {
		log.Println("[行程开始] 防抖到期，但条件不满足，跳过推送")
		return
	}

	c.tripStartNotified = true
	now := time.Now().Local().Format("15:04")

	// 获取最新位置并调用高德 API
	address := "定位获取失败"
	if c.cfg.AmapAPIKey != "" {
		pos, err := db.GetLatestPosition(c.cfg.CarID)
		if err == nil && pos.Latitude != 0 && pos.Longitude != 0 {
			amapClient := geocode.NewAmapClient(c.cfg.AmapAPIKey)
			address = amapClient.GetAddress(pos.Latitude, pos.Longitude)
			log.Printf("[行程开始] 获取地址: %s", address)
		} else {
			log.Printf("[行程开始] 获取位置失败: %v", err)
		}
	}

	title := fmt.Sprintf("🚗 %s 行程开始 📍", c.carName)
	content := fmt.Sprintf(`时间: %s｜表显: %.1f km｜电量: %.0f%%
位置: %s`,
		now, c.lastIdealRangeKM, c.lastBatteryLevel, address,
	)

	log.Printf("[行程开始] 推送通知: %s", title)

	go func() {
		if err := notifier.SendNotification(c.cfg.APIToken, title, content); err != nil {
			log.Printf("❌ 行程开始通知推送失败: %v", err)
		} else {
			log.Println("✅ 行程开始通知已推送")
		}
	}()
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 4: Commit**

```bash
git add internal/mqtt/handler.go
git commit -m "feat: 行程开始通知新增地址显示"
```

---

### Task 6: handler.go 修改行程结束通知

**Files:**
- Modify: `internal/mqtt/handler.go:497-531` (doTripNotification 函数)

- [ ] **Step 1: 修改 doTripNotification 函数**

将 doTripNotification 函数改为：

```go
// 行程通知内容
func (c *Client) doTripNotification(result *db.DriveWithSOC) {
	drive := &result.Drive
	socUsed := result.StartSOC - result.EndSOC
	rangeReduced := drive.StartIdealRangeKM - drive.EndIdealRangeKM
	achieveRate := 0.0
	if rangeReduced > 0 {
		achieveRate = (drive.Distance / rangeReduced) * 100
	}

	// 获取起点和终点地址
	startAddr := "定位获取失败"
	endAddr := "定位获取失败"
	if c.cfg.AmapAPIKey != "" {
		amapClient := geocode.NewAmapClient(c.cfg.AmapAPIKey)
		if result.StartLat != 0 && result.StartLng != 0 {
			startAddr = amapClient.GetAddress(result.StartLat, result.StartLng)
			log.Printf("[行程结束] 获取起点地址: %s", startAddr)
		}
		if result.EndLat != 0 && result.EndLng != 0 {
			endAddr = amapClient.GetAddress(result.EndLat, result.EndLng)
			log.Printf("[行程结束] 获取终点地址: %s", endAddr)
		}
	}

	content := fmt.Sprintf(`时间: %s→%s｜历时: %s｜距离: %.1f km
表显: %.0f→%.0f km｜电量: %.0f→%.0f%%｜达成率: %.1f%%
起点: %s｜终点: %s`,
		drive.StartDate.Local().Format("15:04"), drive.EndDate.Local().Format("15:04"), formatDuration(drive.DurationMin),
		drive.Distance,
		drive.StartIdealRangeKM, drive.EndIdealRangeKM, rangeReduced,
		result.StartSOC, result.EndSOC, socUsed, achieveRate,
		startAddr, endAddr,
	)

	title := fmt.Sprintf("🚗 %s 行程通知 📍", c.carName)

	go func() {
		if err := notifier.SendNotification(c.cfg.APIToken, title, content); err != nil {
			log.Printf("❌ 行程通知推送失败: %v", err)
		} else {
			log.Printf("✅ 行程通知已推送 (ID: %d)", drive.ID)
			c.mu.Lock()
			c.tripStartNotified = false
			c.mu.Unlock()
			log.Println("[行程开始] tripStartNotified 已重置")
		}
	}()
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add internal/mqtt/handler.go
git commit -m "feat: 行程结束通知新增起点终点地址显示"
```

---

### Task 7: 最终验证

- [ ] **Step 1: 完整编译验证**

Run: `go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 2: 检查代码完整性**

Run: `grep -rn "AmapAPIKey\|geocode\|GetLatestPosition\|GetAddress" internal/`
Expected: 应看到所有新增的代码引用

- [ ] **Step 3: 查看完整 diff 确认改动范围**

Run: `git diff HEAD~7`
Expected: 确认改动包含：
1. config.go 新增 AmapAPIKey
2. geocode/convert.go 坐标转换
3. geocode/amap.go 高德 API
4. db.go 新增坐标字段和 GetLatestPosition
5. handler.go 行程开始和结束通知新增地址