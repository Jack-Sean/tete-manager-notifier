package geocode

import "math"

const (
	a  = 6378245.0
	ee = 0.00669342162296594323
)

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