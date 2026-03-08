package cmd

import _ "embed"

//go:embed sources/hackernews.yaml
var hackernewsYAML []byte

//go:embed sources/openmeteo.yaml
var openmeteoYAML []byte

//go:embed sources/coingecko.yaml
var coingeckoYAML []byte

//go:embed sources/worldbank.yaml
var worldbankYAML []byte
