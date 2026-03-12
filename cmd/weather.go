package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/output"
)

var (
	weatherLat      string
	weatherLon      string
	weatherLocation string
)

var weatherCmd = &cobra.Command{
	Use:   "weather",
	Short: "Weather forecasts and current conditions",
	Long: `Fetch weather data from Open-Meteo (no API key required).

Examples:
  wsp weather forecast                         Default location (New York)
  wsp weather forecast --lat=51.5 --lon=-0.1   London weather
  wsp weather current --lat=35.7 --lon=139.7   Tokyo current conditions
  wsp weather forecast --location="Paris"       Geocode by city name
  wsp weather forecast --json                   JSON output`,
}

var weatherForecastCmd = &cobra.Command{
	Use:   "forecast",
	Short: "7-day weather forecast",
	Long: `Get a 7-day weather forecast for a location.

Examples:
  wsp weather forecast                         New York (default)
  wsp weather forecast --lat=48.9 --lon=2.3    Paris
  wsp weather forecast --location="Tokyo"      By city name
  wsp weather forecast --json                  JSON output`,
	RunE: runWeatherAction("forecast"),
}

var weatherCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Current weather conditions",
	Long: `Get current weather conditions for a location.

Examples:
  wsp weather current                          New York (default)
  wsp weather current --lat=51.5 --lon=-0.1   London
  wsp weather current --location="Berlin"      By city name`,
	RunE: runWeatherAction("current"),
}

func init() {
	weatherCmd.PersistentFlags().StringVar(&weatherLat, "lat", "", "Latitude")
	weatherCmd.PersistentFlags().StringVar(&weatherLon, "lon", "", "Longitude")
	weatherCmd.PersistentFlags().StringVar(&weatherLocation, "location", "", "Location name (geocoded)")

	weatherCmd.AddCommand(weatherForecastCmd)
	weatherCmd.AddCommand(weatherCurrentCmd)
}

// weatherCodeToDescription converts WMO weather codes to descriptions.
func weatherCodeToDescription(code float64) string {
	codes := map[int]string{
		0:  "Clear sky",
		1:  "Mainly clear",
		2:  "Partly cloudy",
		3:  "Overcast",
		45: "Foggy",
		48: "Depositing rime fog",
		51: "Light drizzle",
		53: "Moderate drizzle",
		55: "Dense drizzle",
		61: "Slight rain",
		63: "Moderate rain",
		65: "Heavy rain",
		71: "Slight snow",
		73: "Moderate snow",
		75: "Heavy snow",
		80: "Slight rain showers",
		81: "Moderate rain showers",
		82: "Violent rain showers",
		85: "Slight snow showers",
		86: "Heavy snow showers",
		95: "Thunderstorm",
		96: "Thunderstorm with slight hail",
		99: "Thunderstorm with heavy hail",
	}
	if desc, ok := codes[int(code)]; ok {
		return desc
	}
	return "Unknown"
}

func runWeatherAction(action string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		lat, lon, err := resolveLocation()
		if err != nil {
			return err
		}

		// Build Open-Meteo URL directly for better control.
		var url string
		switch action {
		case "forecast":
			url = fmt.Sprintf(
				"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,weather_code&timezone=auto&forecast_days=7",
				lat, lon,
			)
		case "current":
			url = fmt.Sprintf(
				"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m,relative_humidity_2m,apparent_temperature,wind_speed_10m,weather_code&timezone=auto",
				lat, lon,
			)
		}

		if flagDebug {
			fmt.Fprintf(os.Stderr, "[debug] fetching weather from %s\n", url)
		}

		ctx := cmd.Context()
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("User-Agent", "wellspring-cli/0.1")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("fetching weather: %w\n\nHint: check your internet connection and try again", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Open-Meteo returned status %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		var points []adapter.DataPoint
		switch action {
		case "forecast":
			points = parseWeatherForecast(result, lat, lon)
		case "current":
			points = parseWeatherCurrent(result, lat, lon)
		}

		output.Render(os.Stdout, points, getOutputFormat(), flagNoColor)
		return nil
	}
}

func parseWeatherForecast(result map[string]any, lat, lon string) []adapter.DataPoint {
	daily, ok := result["daily"].(map[string]any)
	if !ok {
		return nil
	}

	times, _ := daily["time"].([]any)
	maxTemps, _ := daily["temperature_2m_max"].([]any)
	minTemps, _ := daily["temperature_2m_min"].([]any)
	precip, _ := daily["precipitation_sum"].([]any)
	weatherCodes, _ := daily["weather_code"].([]any)

	points := make([]adapter.DataPoint, 0, len(times))
	for i, t := range times {
		dateStr, _ := t.(string)
		date, _ := time.Parse("2006-01-02", dateStr)

		maxTemp := getFloatFromAny(maxTemps, i)
		minTemp := getFloatFromAny(minTemps, i)
		precipVal := getFloatFromAny(precip, i)
		code := getFloatFromAny(weatherCodes, i)

		dp := adapter.DataPoint{
			Source:   "openmeteo",
			Category: "weather",
			Title:   fmt.Sprintf("%s  %s", dateStr, weatherCodeToDescription(code)),
			Value:   fmt.Sprintf("%.1f°C / %.1f°C", maxTemp, minTemp),
			Time:    date,
			Meta: map[string]any{
				"temp_max":      maxTemp,
				"temp_min":      minTemp,
				"precipitation": precipVal,
				"weather_code":  code,
				"description":   weatherCodeToDescription(code),
				"latitude":      lat,
				"longitude":     lon,
			},
		}
		points = append(points, dp)
	}
	return points
}

func parseWeatherCurrent(result map[string]any, lat, lon string) []adapter.DataPoint {
	current, ok := result["current"].(map[string]any)
	if !ok {
		return nil
	}

	temp := getMapFloat(current, "temperature_2m")
	humidity := getMapFloat(current, "relative_humidity_2m")
	feelsLike := getMapFloat(current, "apparent_temperature")
	wind := getMapFloat(current, "wind_speed_10m")
	code := getMapFloat(current, "weather_code")

	dp := adapter.DataPoint{
		Source:   "openmeteo",
		Category: "weather",
		Title:   fmt.Sprintf("%.1f°C — %s", temp, weatherCodeToDescription(code)),
		Value:   temp,
		Time:    time.Now(),
		Meta: map[string]any{
			"temperature":        temp,
			"feels_like":         feelsLike,
			"humidity":           humidity,
			"wind_speed":         wind,
			"weather_code":       code,
			"description":        weatherCodeToDescription(code),
			"latitude":           lat,
			"longitude":          lon,
		},
	}
	return []adapter.DataPoint{dp}
}

func resolveLocation() (lat, lon string, err error) {
	// If explicit lat/lon provided, use them.
	if weatherLat != "" && weatherLon != "" {
		return weatherLat, weatherLon, nil
	}

	// If location name provided, geocode it.
	if weatherLocation != "" {
		return geocode(weatherLocation)
	}

	// Default: New York.
	return "40.7128", "-74.0060", nil
}

func geocode(location string) (lat, lon string, err error) {
	url := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=en&format=json",
		strings.ReplaceAll(location, " ", "+"))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("geocoding %q: %w\n\nHint: try using --lat and --lon instead", location, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Name      string  `json:"name"`
			Country   string  `json:"country"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("parsing geocoding response: %w", err)
	}

	if len(result.Results) == 0 {
		return "", "", fmt.Errorf("location %q not found\n\nHint: try a different spelling or use --lat and --lon", location)
	}

	r := result.Results[0]
	if flagDebug {
		fmt.Fprintf(os.Stderr, "[debug] geocoded %q → %s, %s (%.4f, %.4f)\n",
			location, r.Name, r.Country, r.Latitude, r.Longitude)
	}

	return strconv.FormatFloat(r.Latitude, 'f', 4, 64),
		strconv.FormatFloat(r.Longitude, 'f', 4, 64), nil
}

func getFloatFromAny(arr []any, i int) float64 {
	if i >= len(arr) {
		return 0
	}
	if f, ok := arr[i].(float64); ok {
		return f
	}
	return 0
}

func getMapFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}
