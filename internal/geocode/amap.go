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

func (c *AmapClient) GetAddress(lat, lng float64) string {
	if c.apiKey == "" {
		return "定位获取失败"
	}
	gcjLat, gcjLng := WGS84ToGCJ02(lat, lng)
	resp, err := c.reverseGeocode(gcjLat, gcjLng)
	if err != nil {
		log.Printf("[高德API] 调用失败: %v", err)
		return "定位获取失败"
	}
	return formatAddress(resp)
}

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

func formatAddress(resp *RegeoResponse) string {
	comp := resp.Regeocode.AddressComponent
	district := strings.TrimSuffix(comp.District, "区")
	if len(resp.Regeocode.Pois) > 0 && resp.Regeocode.Pois[0].Name != "" {
		return district + resp.Regeocode.Pois[0].Name
	}
	if comp.Street != "" && comp.StreetNumber != "" {
		return district + comp.Street + comp.StreetNumber
	}
	if comp.Street != "" {
		return district + comp.Street
	}
	if district != "" {
		return district
	}
	return "定位获取失败"
}

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