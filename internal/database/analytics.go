package database

import "time"

// AnalyticalReport represents the results of an analytical query
type AnalyticalReport struct {
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	GeneratedAt time.Time              `json:"generated_at"`
	QueryTime   time.Duration          `json:"query_time"`
	Results     []AnalyticalResult     `json:"results"`
	Summary     map[string]interface{} `json:"summary,omitempty"`
	ChartData   *ChartData             `json:"chart_data,omitempty"` // For graph visualization
}

// AnalyticalResult represents a single result row from an analytical query
type AnalyticalResult struct {
	Date     *time.Time             `json:"date,omitempty"`
	Zone     *int                   `json:"zone,omitempty"`
	Net      *int                   `json:"net,omitempty"`
	Node     *int                   `json:"node,omitempty"`
	Value    string                 `json:"value"`
	Count    int                    `json:"count,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ChartData represents data formatted for chart visualization
type ChartData struct {
	Type       string            `json:"type"`        // "line", "bar", "pie", "area"
	Title      string            `json:"title"`
	XAxisLabel string            `json:"x_axis_label"`
	YAxisLabel string            `json:"y_axis_label"`
	Series     []ChartSeries     `json:"series"`
	Categories []string          `json:"categories,omitempty"` // For categorical data
	Colors     []string          `json:"colors,omitempty"`     // Custom colors
	Options    map[string]interface{} `json:"options,omitempty"` // Chart-specific options
}

// ChartSeries represents a data series for charts
type ChartSeries struct {
	Name string        `json:"name"`
	Data []ChartPoint  `json:"data"`
}

// ChartPoint represents a single data point in a chart
type ChartPoint struct {
	X     interface{} `json:"x"` // Date, string, or number
	Y     int         `json:"y"`
	Label string      `json:"label,omitempty"`
}

// V34ModemReport represents analysis of V.34 modem adoption in FidoNet
type V34ModemReport struct {
	FirstAppearance         *AnalyticalResult     `json:"first_appearance"`
	EarliestAdopters        []AnalyticalResult    `json:"earliest_adopters"`
	AdoptionOverTime        []YearlyCount         `json:"adoption_over_time"`
	TopAdoptingZones        []ZoneCount           `json:"top_adopting_zones"`
	TotalNodesWithV34       int                   `json:"total_nodes_with_v34"`
	TotalNodesAnalyzed      int                   `json:"total_nodes_analyzed"`
	AdoptionPercentage      float64               `json:"adoption_percentage"`
	ChartData               *ChartData            `json:"chart_data,omitempty"`
}

// BinkpReport represents analysis of Binkp protocol introduction
type BinkpReport struct {
	FirstAppearance         *AnalyticalResult     `json:"first_appearance"`
	EarliestAdopters        []AnalyticalResult    `json:"earliest_adopters"`
	AdoptionOverTime        []YearlyCount         `json:"adoption_over_time"`
	TopAdoptingZones        []ZoneCount           `json:"top_adopting_zones"`
	TotalNodesWithBinkp     int                   `json:"total_nodes_with_binkp"`
	TotalNodesAnalyzed      int                   `json:"total_nodes_analyzed"`
	AdoptionPercentage      float64               `json:"adoption_percentage"`
	ChartData               *ChartData            `json:"chart_data,omitempty"`
}

// NetworkLifecycleReport represents analysis of network creation and deletion
type NetworkLifecycleReport struct {
	NetworkAddress    string                `json:"network_address"`
	Zone              int                   `json:"zone"`
	Net               int                   `json:"net"`
	HostName          string                `json:"host_name,omitempty"`
	FirstSeen         time.Time             `json:"first_seen"`
	LastSeen          time.Time             `json:"last_seen"`
	TotalDays         int                   `json:"total_days"`
	MaxNodes          int                   `json:"max_nodes"`
	MaxNodesDate      time.Time             `json:"max_nodes_date"`
	History           []NetworkHistory      `json:"history"`
	Status            string                `json:"status"` // "active", "inactive"
	IsCurrentlyActive bool                  `json:"is_currently_active"`
	ChartData         *ChartData            `json:"chart_data,omitempty"`
}

// ZoneCount represents a count for a specific zone
type ZoneCount struct {
	Zone  int `json:"zone"`
	Count int `json:"count"`
}

// NameCount represents a count for a specific name
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// NetworkHistory represents historical node count for a network
type NetworkHistory struct {
	Date      time.Time `json:"date"`
	NodeCount int       `json:"node_count"`
}

// SysopNameReport represents sysop name analysis by year
type SysopNameReport struct {
	Year        int           `json:"year"`
	TopNames    []NameCount   `json:"top_names"`
	TotalUnique int           `json:"total_unique"`
	TotalNodes  int           `json:"total_nodes"`
	ChartData   *ChartData    `json:"chart_data,omitempty"`
}

// YearlyCount represents a count for a specific year
type YearlyCount struct {
	Year  int `json:"year"`
	Count int `json:"count"`
}

// TrendAnalysis represents trend data over time
type TrendAnalysis struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	DataPoints  []TrendDataPoint    `json:"data_points"`
	StartDate   time.Time           `json:"start_date"`
	EndDate     time.Time           `json:"end_date"`
	Trend       string              `json:"trend"`       // "increasing", "decreasing", "stable"
	ChartData   *ChartData          `json:"chart_data,omitempty"`
}

// TrendDataPoint represents a single point in a trend analysis
type TrendDataPoint struct {
	Date  time.Time `json:"date"`
	Value int       `json:"value"`
	Label string    `json:"label,omitempty"`
}

// ProtocolAdoptionReport represents protocol adoption analysis
type ProtocolAdoptionReport struct {
	Protocol            string            `json:"protocol"`
	FirstAppearance     *time.Time        `json:"first_appearance"`
	EarliestAdopters    []AnalyticalResult `json:"earliest_adopters"`
	AdoptionOverTime    []YearlyCount     `json:"adoption_over_time"`
	TopAdoptingZones    []ZoneCount       `json:"top_adopting_zones"`
	CurrentAdoption     int               `json:"current_adoption"`
	TotalNodes          int               `json:"total_nodes"`
	AdoptionPercentage  float64           `json:"adoption_percentage"`
	ChartData           *ChartData        `json:"chart_data,omitempty"`
}

// GeographicDistribution represents geographic analysis
type GeographicDistribution struct {
	Region      string     `json:"region"`
	ZoneNumber  int        `json:"zone_number"`
	NodeCount   int        `json:"node_count"`
	Percentage  float64    `json:"percentage"`
	ChartData   *ChartData `json:"chart_data,omitempty"`
}

// TechnologyEvolution represents how technology has evolved over time
type TechnologyEvolution struct {
	Technology       string              `json:"technology"`
	EvolutionPoints  []TrendDataPoint    `json:"evolution_points"`
	PeakAdoption     *TrendDataPoint     `json:"peak_adoption"`
	CurrentStatus    string              `json:"current_status"` // "growing", "declining", "stable", "obsolete"
	ChartData        *ChartData          `json:"chart_data,omitempty"`
}