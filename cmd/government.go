package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/output"
)

var (
	govCountry   string
	govIndicator string
)

var governmentCmd = &cobra.Command{
	Use:   "government",
	Short: "Government and economic data from World Bank",
	Long: `Fetch government and economic data from the World Bank API (no API key required).

Examples:
  wsp government population --country=US     US population over time
  wsp government gdp --country=CN            China GDP over time
  wsp government indicators --country=BR --indicator=SP.POP.TOTL
  wsp government countries                   List all countries
  wsp government population --json            JSON output`,
}

var govPopulationCmd = &cobra.Command{
	Use:   "population",
	Short: "Population data by country",
	Long: `Fetch population data for a specific country over time.

Examples:
  wsp government population --country=US     United States
  wsp government population --country=IN     India
  wsp government population --country=WLD    World total`,
	RunE: runGovIndicator("SP.POP.TOTL", "Population"),
}

var govGDPCmd = &cobra.Command{
	Use:   "gdp",
	Short: "GDP data by country",
	Long: `Fetch GDP data for a specific country over time.

Examples:
  wsp government gdp --country=US            United States GDP
  wsp government gdp --country=CN            China GDP
  wsp government gdp --country=JP            Japan GDP`,
	RunE: runGovIndicator("NY.GDP.MKTP.CD", "GDP (current US$)"),
}

var govIndicatorsCmd = &cobra.Command{
	Use:   "indicators",
	Short: "Custom World Bank indicator",
	Long: `Fetch any World Bank indicator by code.

Examples:
  wsp government indicators --country=US --indicator=SP.POP.TOTL
  wsp government indicators --country=US --indicator=NY.GDP.MKTP.CD

Common indicators:
  SP.POP.TOTL         Total population
  NY.GDP.MKTP.CD      GDP (current US$)
  NY.GDP.PCAP.CD      GDP per capita
  SL.UEM.TOTL.ZS      Unemployment rate
  FP.CPI.TOTL.ZG      Inflation (CPI)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if govIndicator == "" {
			return fmt.Errorf("--indicator is required\n\nCommon indicators:\n  SP.POP.TOTL     Population\n  NY.GDP.MKTP.CD  GDP\n  NY.GDP.PCAP.CD  GDP per capita")
		}
		return runGovIndicator(govIndicator, govIndicator)(cmd, args)
	},
}

var govCountriesCmd = &cobra.Command{
	Use:   "countries",
	Short: "List available countries",
	Long: `List all countries available in the World Bank API.

Examples:
  wsp government countries                   List all countries
  wsp government countries --limit=50        List 50 countries`,
	RunE: runGovCountries,
}

func init() {
	governmentCmd.PersistentFlags().StringVar(&govCountry, "country", "US", "Country code (ISO 3166-1 alpha-2, e.g., US, CN, IN)")
	governmentCmd.PersistentFlags().StringVar(&govIndicator, "indicator", "", "World Bank indicator code")

	governmentCmd.AddCommand(govPopulationCmd)
	governmentCmd.AddCommand(govGDPCmd)
	governmentCmd.AddCommand(govIndicatorsCmd)
	governmentCmd.AddCommand(govCountriesCmd)
}

func runGovIndicator(indicator, label string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		country := govCountry
		if country == "" {
			country = "US"
		}

		url := fmt.Sprintf("https://api.worldbank.org/v2/country/%s/indicator/%s?format=json&per_page=%d&date=2015:2024",
			country, indicator, flagLimit)

		if flagDebug {
			fmt.Fprintf(os.Stderr, "[debug] fetching from World Bank: %s\n", url)
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
			return fmt.Errorf("fetching data: %w\n\nHint: check your internet connection", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		// World Bank returns array: [metadata, data]
		var raw []json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			return fmt.Errorf("parsing response: %w\n\nThe World Bank API may be temporarily unavailable", err)
		}

		if len(raw) < 2 {
			return fmt.Errorf("unexpected response format from World Bank API")
		}

		var entries []worldBankEntry
		if err := json.Unmarshal(raw[1], &entries); err != nil {
			return fmt.Errorf("parsing data entries: %w", err)
		}

		points := make([]adapter.DataPoint, 0, len(entries))
		for _, entry := range entries {
			t, _ := time.Parse("2006", entry.Date)

			var value any
			if entry.Value != nil {
				value = *entry.Value
			}

			dp := adapter.DataPoint{
				Source:   "worldbank",
				Category: "government",
				Title:   fmt.Sprintf("%s — %s (%s)", entry.Country.Value, label, entry.Date),
				Value:   value,
				Time:    t,
				Meta: map[string]any{
					"country":    entry.Country.Value,
					"country_id": entry.CountryISO3,
					"indicator":  entry.Indicator.Value,
					"year":       entry.Date,
				},
			}
			points = append(points, dp)
		}

		output.Render(os.Stdout, points, getOutputFormat(), flagNoColor)
		return nil
	}
}

func runGovCountries(cmd *cobra.Command, args []string) error {
	url := fmt.Sprintf("https://api.worldbank.org/v2/country?format=json&per_page=%d", flagLimit)

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "wellspring-cli/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching countries: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(raw) < 2 {
		return fmt.Errorf("unexpected response format")
	}

	var countries []struct {
		ID        string `json:"id"`
		ISO2Code  string `json:"iso2Code"`
		Name      string `json:"name"`
		Region    struct {
			Value string `json:"value"`
		} `json:"region"`
		IncomeLevel struct {
			Value string `json:"value"`
		} `json:"incomeLevel"`
		CapitalCity string `json:"capitalCity"`
	}

	if err := json.Unmarshal(raw[1], &countries); err != nil {
		return fmt.Errorf("parsing countries: %w", err)
	}

	points := make([]adapter.DataPoint, 0, len(countries))
	for _, c := range countries {
		dp := adapter.DataPoint{
			Source:   "worldbank",
			Category: "government",
			Title:   fmt.Sprintf("%s (%s)", c.Name, c.ISO2Code),
			Value:   c.ISO2Code,
			Time:    time.Now(),
			Meta: map[string]any{
				"id":           c.ID,
				"iso2":         c.ISO2Code,
				"region":       c.Region.Value,
				"income_level": c.IncomeLevel.Value,
				"capital":      c.CapitalCity,
			},
		}
		points = append(points, dp)
	}

	output.Render(os.Stdout, points, getOutputFormat(), flagNoColor)
	return nil
}

type worldBankEntry struct {
	Indicator struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"indicator"`
	Country struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"country"`
	CountryISO3 string   `json:"countryiso3code"`
	Date        string   `json:"date"`
	Value       *float64 `json:"value"`
	Unit        string   `json:"unit"`
	Decimal     int      `json:"decimal"`
}

func formatLargeNumber(n float64) string {
	switch {
	case n >= 1e12:
		return fmt.Sprintf("%.2fT", n/1e12)
	case n >= 1e9:
		return fmt.Sprintf("%.2fB", n/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.2fM", n/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.2fK", n/1e3)
	default:
		return strconv.FormatFloat(n, 'f', 2, 64)
	}
}
