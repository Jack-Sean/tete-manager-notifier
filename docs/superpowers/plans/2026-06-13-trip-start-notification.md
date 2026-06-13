# 行程开始通知 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 当车辆 state 变为 "driving" 时，推送行程开始通知到「特特管家」App。

**Architecture:** 在 `internal/mqtt/handler.go` 中，监听 state 变化，检测到 driving 后启动防抖计时器，防抖到期确认仍在行驶则推送通知。新增 `tripStartNotified` 标志防止重复通知，在行程结束通知推送后重置。

**Tech Stack:** Go, paho.mqtt.golang

**Spec:** `docs/superpowers/specs/2026-06-13-trip-start-notification-design.md`

---

### Task 1: Client 结构体新增字段并初始化

**Files:**
- Modify: `internal/mqtt/handler.go:20-42` (Client 结构体)
- Modify: `internal/mqtt/handler.go:44-55` (NewClient 函数)

- [ ] **Step 1: 在 Client 结构体中新增两个字段**

在 `Client` 结构体中，`retryCountCharge int` 之后添加：

```go
	tripStartNotified      bool
	tripStartDebounceTimer *time.Timer
```

完整结构体变为：

```go
type Client struct {
	cfg                *config.Config
	client             mqtt.Client
	carName            string
	lastDriveID        uint
	lastChargeID       uint
	lastShiftState     string
	lastUserPresent    bool
	lastChargingState  string
	lastBatteryLevel   float64
	lastIdealRangeKM   float64
	lastDriverDoorOpen bool
	lastLocked         bool
	lastState          string
	chargeLimitSoc     int

	mu               sync.Mutex
	debounceTimer    *time.Timer
	stateSettleTimer *time.Timer

	retryCountDrive  int
	retryCountCharge int

	tripStartNotified      bool
	tripStartDebounceTimer *time.Timer
}
```

- [ ] **Step 2: 在 NewClient 中初始化新字段**

在 `NewClient` 函数的 return 语句中，`lastState: ""` 之后添加：

```go
		tripStartNotified: false,
```

完整 NewClient 变为：

```go
func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:               cfg,
		lastDriveID:       0,
		lastChargeID:      0,
		lastUserPresent:   true,
		lastChargingState: "",
		lastBatteryLevel:  0,
		lastIdealRangeKM:  0,
		lastState:         "",
		tripStartNotified: false,
	}
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 4: Commit**

```bash
git add internal/mqtt/handler.go
git commit -m "feat: 新增 tripStartNotified 和 tripStartDebounceTimer 字段"
```

---

### Task 2: messageHandler state 分支加入防抖触发

**Files:**
- Modify: `internal/mqtt/handler.go:134-140` (messageHandler 中的 state 分支)

- [ ] **Step 1: 修改 state 分支，加入行程开始防抖逻辑**

将 messageHandler 中原来的 state 分支：

```go
	case strings.HasSuffix(topic, "/state"):
		c.mu.Lock()
		if c.lastState != payload {
			c.lastState = payload
			isDrive = true
		}
		c.mu.Unlock()
```

替换为：

```go
	case strings.HasSuffix(topic, "/state"):
		c.mu.Lock()
		if c.lastState != payload {
			c.lastState = payload
			isDrive = true

			if payload == "driving" && !c.tripStartNotified {
				if c.tripStartDebounceTimer != nil {
					c.tripStartDebounceTimer.Stop()
				}
				c.tripStartDebounceTimer = time.AfterFunc(
					time.Duration(c.cfg.PushDebounceSec)*time.Second,
					c.processTripStart,
				)
				log.Printf("[行程开始] state 变为 driving，启动 %d 秒防抖", c.cfg.PushDebounceSec)
			} else if payload != "driving" {
				if c.tripStartDebounceTimer != nil {
					c.tripStartDebounceTimer.Stop()
					c.tripStartDebounceTimer = nil
				}
				log.Printf("[行程开始] state 变为 %s，取消防抖", payload)
			}
		}
		c.mu.Unlock()
```

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 编译失败（`processTripStart` 方法尚未定义），确认错误信息为 `c.processTripStart undefined`

> 注意：这里预期编译失败是正常的，因为 `processTripStart` 方法在 Task 3 中才添加。如果你希望每一步都编译通过，可以先添加一个空的 `processTripStart` 方法占位。

- [ ] **Step 3: Commit**

```bash
git add internal/mqtt/handler.go
git commit -m "feat: state 分支加入行程开始防抖触发逻辑"
```

---

### Task 3: 实现 processTripStart 方法

**Files:**
- Modify: `internal/mqtt/handler.go` (在 `processChargeStart` 方法之前添加新方法)

- [ ] **Step 1: 添加 processTripStart 方法**

在 `checkChargingCondition` 方法之前（即 `scheduleStateSettle` 方法之后），添加以下方法：

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

	title := fmt.Sprintf("🚗 %s 行程开始 📍", c.carName)
	content := fmt.Sprintf(`时间: %s
电量: %.0f%%
表显: %.1f km`,
		now, c.lastBatteryLevel, c.lastIdealRangeKM,
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

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 3: Commit**

```bash
git add internal/mqtt/handler.go
git commit -m "feat: 实现 processTripStart 行程开始通知方法"
```

---

### Task 4: processTripEnd 中重置 tripStartNotified

**Files:**
- Modify: `internal/mqtt/handler.go:469-475` (doTripNotification 方法)

- [ ] **Step 1: 在 doTripNotification 推送通知后重置标志**

在 `doTripNotification` 方法中，找到推送通知的 goroutine 的 `else` 分支（推送成功时），在 `log.Printf("✅ 行程通知已推送 (ID: %d)", drive.ID)` 之后添加重置逻辑。

将 `doTripNotification` 方法末尾的 goroutine：

```go
	go func() {
		if err := notifier.SendNotification(c.cfg.APIToken, title, content); err != nil {
			log.Printf("❌ 行程通知推送失败: %v", err)
		} else {
			log.Printf("✅ 行程通知已推送 (ID: %d)", drive.ID)
		}
	}()
```

替换为：

```go
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
```

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 3: Commit**

```bash
git add internal/mqtt/handler.go
git commit -m "feat: 行程结束通知推送后重置 tripStartNotified 标志"
```

---

### Task 5: 最终验证

- [ ] **Step 1: 完整编译验证**

Run: `go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 2: 检查代码完整性**

Run: `grep -n "tripStart" internal/mqtt/handler.go`
Expected: 应看到以下关键行：
- `tripStartNotified` 字段声明
- `tripStartDebounceTimer` 字段声明
- `tripStartNotified: false` 初始化
- `c.processTripStart` 防抖回调引用
- `func (c *Client) processTripStart()` 方法定义
- `c.tripStartNotified = true` 推送时设置
- `c.tripStartNotified = false` 行程结束时重置

- [ ] **Step 3: 查看完整 diff 确认改动范围**

Run: `git diff HEAD~4 -- internal/mqtt/handler.go`
Expected: 改动仅限于 handler.go，确认：
1. Client 结构体新增两个字段
2. NewClient 初始化新字段
3. state 分支加入防抖逻辑
4. 新增 processTripStart 方法
5. doTripNotification 中重置标志
