# 行程开始通知 - 设计文档

## 目标

在车辆开始行驶时，实时推送一条通知到「特特管家」App，告知用户行程已开始。

## 触发条件

监听 MQTT topic `teslamate/cars/{CAR_ID}/state`，当 `state` 从非 `driving` 变为 `driving` 时触发。

### 防抖

- 复用 `PUSH_DEBOUNCE_SECONDS` 环境变量
- 启动独立防抖计时器 `tripStartDebounceTimer`
- 防抖到期后再次确认 `state == "driving"`，确认后才推送
- 防抖期间若 state 变回非 driving，取消计时器

### 防重复通知

- 新增字段 `tripStartNotified bool`
- 推送成功后设为 `true`，本次行程不再重复推送
- 在 `checkTripEndCondition()` 行程结束流程中重置为 `false`

## 通知内容

```
🚗 {车辆名} 行程开始 📍
时间: 14:30
电量: 80%
表显: 320.5 km
```

| 字段 | 来源 | 说明 |
|------|------|------|
| 车辆名 | `c.carName` | MQTT 连接时从数据库获取 |
| 时间 | `time.Now()` | 当前时间，格式 HH:MM |
| 电量 | `c.lastBatteryLevel` | MQTT 实时值 |
| 表显 | `c.lastIdealRangeKM` | MQTT 实时值，单位 km |

不需要查询数据库，全部使用 MQTT 实时状态。

## 代码改动

### 文件：`internal/mqtt/handler.go`

**Client 结构体新增字段：**

```go
tripStartNotified     bool
tripStartDebounceTimer *time.Timer
```

**messageHandler 中 state 分支新增逻辑：**

```go
case strings.HasSuffix(topic, "/state"):
    c.mu.Lock()
    if c.lastState != payload {
        c.lastState = payload
        isDrive = true

        if payload == "driving" && !c.tripStartNotified {
            // state 变为 driving，启动行程开始防抖
            if c.tripStartDebounceTimer != nil {
                c.tripStartDebounceTimer.Stop()
            }
            c.tripStartDebounceTimer = time.AfterFunc(
                time.Duration(c.cfg.PushDebounceSec)*time.Second,
                c.processTripStart,
            )
        } else if payload != "driving" {
            // state 变为非 driving，取消防抖
            if c.tripStartDebounceTimer != nil {
                c.tripStartDebounceTimer.Stop()
                c.tripStartDebounceTimer = nil
            }
        }
    }
    c.mu.Unlock()
```

**新增 processTripStart 方法：**

```go
func (c *Client) processTripStart() {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.lastState != "driving" || c.tripStartNotified {
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

    go func() {
        if err := notifier.SendNotification(c.cfg.APIToken, title, content); err != nil {
            log.Printf("❌ 行程开始通知推送失败: %v", err)
        } else {
            log.Println("✅ 行程开始通知已推送")
        }
    }()
}
```

**processTripEnd 中重置标志：**

在行程通知实际推送**之后**加入：

```go
c.tripStartNotified = false
```

确保只有在行程通知成功推送后，下一次行程开始才能重新触发通知。避免通知失败时过早重置导致状态不一致。

## 不影响现有逻辑

- 不修改 `scheduleStateSettle()`、`checkTripEndCondition()` 的现有判断逻辑
- 不修改 `processChargeStart()`、`processChargeEnd()` 等现有方法
- 仅在 state 分支中加入行程开始的防抖触发
- 仅在 `processTripEnd()` 通知推送**之后**重置 `tripStartNotified = false`
