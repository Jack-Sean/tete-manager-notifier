# 行程地址显示 - 设计文档

## 目标

为行程开始和行程结束通知增加中文地址显示，使用高德地图逆地理编码 API 将 GPS 坐标转换为简短地址。

## 通知格式

### 行程开始

```
🚗 我的Tesla 行程开始 📍
时间: 14:30｜表显: 320.5 km｜电量: 80%
位置: 萧山万象汇
```

### 行程结束

```
🚗 我的Tesla 行程通知 📍
时间: 14:30→15:20｜历时: 50min｜距离: 35.2 km
表显: 320→280 km｜电量: 80→65%｜达成率: 88.2%
起点: 萧山万象汇｜终点: 滨江龙湖天街
```

## 地址格式

示例：`萧山万象汇`、`萧山北干初中`、`朝阳望京SOHO`

组合逻辑：
1. 有 POI → `district`（去掉「区」字）+ `pois[0].name`
2. 无 POI → `district` + `street`
3. API 失败 → 显示「定位获取失败」

## 配置新增

| 环境变量 | 说明 | 必填 | 默认值 |
|---------|------|------|--------|
| `AMAP_API_KEY` | 高德 Web 服务 API Key | 是 | - |

在 `docker-compose.yml` 中新增：
```yaml
- AMAP_API_KEY=你的高德API密钥
```

## 高德 API

逆地理编码接口：
```
GET https://restapi.amap.com/v3/geocode/regeo?key={API_KEY}&location={lng},{lat}&extensions=all
```

返回示例：
```json
{
  "status": "1",
  "regeocode": {
    "addressComponent": {
      "district": "萧山区",
      "street": "市心北路"
    },
    "pois": [
      { "name": "萧山万象汇" }
    ]
  }
}
```

## 数据来源

### 行程开始地址

- 从 `positions` 表查询该车最新一条记录的 `latitude` 和 `longitude`
- 新增 `db.GetLatestPosition(carID)` 函数

### 行程结束地址

- `GetLatestDrive` 已关联 `positions` 表
- 新增返回字段：`start_lat`, `start_lng`, `end_lat`, `end_lng`

## 错误处理

- API 调用失败：地址显示「定位获取失败」
- 无坐标数据：地址显示「定位获取失败」
- 通知仍正常发送，不影响其他信息

## 代码改动

### 新增文件

| 文件 | 说明 |
|------|------|
| `internal/geocode/amap.go` | 高德 API 调用封装，地址格式化逻辑 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `internal/config/config.go` | 新增 `AmapAPIKey string` 字段 |
| `internal/db/db.go` | `GetLatestDrive` 新增返回坐标字段；新增 `GetLatestPosition` 函数 |
| `internal/mqtt/handler.go` | `processTripStart` 调用地址 API；`doTripNotification` 调用地址 API；调整通知格式 |

## geocode/amap.go 设计

```go
package geocode

type AmapClient struct {
    apiKey string
}

func NewAmapClient(apiKey string) *AmapClient

// GetAddress 获取简短地址（如"萧山万象汇"）
func (c *AmapClient) GetAddress(lat, lng float64) string

// 内部方法：调用高德 API
func (c *AmapClient) reverseGeocode(lat, lng float64) (*RegeoResponse, error)

// 内部方法：格式化地址
func formatAddress(resp *RegeoResponse) string
```

## db/db.go 改动

### GetLatestDrive 新增字段

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

SQL 新增查询字段：
```sql
start_pos.latitude as start_lat,
start_pos.longitude as start_lng,
end_pos.latitude as end_lat,
end_pos.longitude as end_lng
```

### 新增 GetLatestPosition

```go
type Position struct {
    Latitude  float64
    Longitude float64
}

func GetLatestPosition(carID int) (*Position, error)
```

## handler.go 改动

### processTripStart

```go
func (c *Client) processTripStart() {
    // 获取最新位置
    pos, err := db.GetLatestPosition(c.cfg.CarID)
    // 调用地址 API
    address := geocodeClient.GetAddress(pos.Latitude, pos.Longitude)

    title := fmt.Sprintf("🚗 %s 行程开始 📍", c.carName)
    content := fmt.Sprintf(`时间: %s｜表显: %.1f km｜电量: %.0f%%
位置: %s`,
        now, c.lastIdealRangeKM, c.lastBatteryLevel, address,
    )
    // ...
}
```

### doTripNotification

```go
func (c *Client) doTripNotification(result *db.DriveWithSOC) {
    // 获取起点和终点地址
    startAddr := geocodeClient.GetAddress(result.StartLat, result.StartLng)
    endAddr := geocodeClient.GetAddress(result.EndLat, result.EndLng)

    content := fmt.Sprintf(`时间: %s→%s｜历时: %s｜距离: %.1f km
表显: %.0f→%.0f km｜电量: %.0f→%.0f%%｜达成率: %.1f%%
起点: %s｜终点: %s`,
        ..., startAddr, endAddr,
    )
    // ...
}
```