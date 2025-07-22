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

// V34ModemReport represents the first V.34 modem appearance analysis
type V34ModemReport struct {
	FirstAppearance time.Time `json:"first_appearance"`
	FirstNode       NodeInfo  `json:"first_node"`
	TotalV34Nodes   int       `json:"total_v34_nodes"`
	AdoptionByYear  []YearlyCount `json:"adoption_by_year"`
	ChartData       *ChartData    `json:"chart_data,omitempty"`
}

// BinkpReport represents Binkp protocol introduction analysis
type BinkpReport struct {
	FirstAppearance time.Time     `json:"first_appearance"`
	FirstNode       NodeInfo      `json:"first_node"`
	TotalBinkpNodes int           `json:"total_binkp_nodes"`
	AdoptionByYear  []YearlyCount `json:"adoption_by_year"`
	ChartData       *ChartData    `json:"chart_data,omitempty"`
}

// NetworkLifecycleReport represents network creation/deletion analysis
type NetworkLifecycleReport struct {
	Zone        int              `json:"zone"`
	Net         int              `json:"net"`
	HostName    string           `json:"host_name,omitempty"`
	FirstSeen   time.Time        `json:"first_seen"`
	LastSeen    time.Time        `json:"last_seen"`
	IsActive    bool             `json:"is_active"`
	Duration    string           `json:"duration"` // Human readable duration
	MaxNodes    int              `json:"max_nodes"`
	NodeHistory []NetworkHistory `json:"node_history"`
	ChartData   *ChartData       `json:"chart_data,omitempty"`
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

// NameCount represents a name and its frequency
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// NodeInfo represents basic node information for reports
type NodeInfo struct {
	Zone       int    `json:"zone"`
	Net        int    `json:"net"`
	Node       int    `json:"node"`
	Address    string `json:"address"`
	SystemName string `json:"system_name,omitempty"`
	SysopName  string `json:"sysop_name,omitempty"`
	Location   string `json:"location,omitempty"`
}

// TrendAnalysis represents trend data over time for graphing
type TrendAnalysis struct {
	Metric     string       `json:"metric"`
	TimePoints []TrendPoint `json:"time_points"`
	Summary    struct {
		PeakDate  time.Time `json:"peak_date"`
		PeakValue int       `json:"peak_value"`
		Growth    string    `json:"growth"` // "increasing", "decreasing", "stable"
		StartDate time.Time `json:"start_date"`
		EndDate   time.Time `json:"end_date"`
	} `json:"summary"`
	ChartData *ChartData `json:"chart_data,omitempty"`
}

// TrendPoint represents a single point in trend analysis
type TrendPoint struct {
	Date  time.Time `json:"date"`
	Value int       `json:"value"`
}

// ProtocolAdoptionReport represents adoption of various protocols over time
type ProtocolAdoptionReport struct {
	Protocol    string        `json:"protocol"`
	Timeline    []TrendPoint  `json:"timeline"`
	PeakAdoption TrendPoint   `json:"peak_adoption"`
	FirstSeen    time.Time    `json:"first_seen"`
	ChartData    *ChartData   `json:"chart_data,omitempty"`
}

// GeographicDistribution represents geographic analysis of FidoNet
type GeographicDistribution struct {
	Region     string     `json:"region"`
	NodeCounts []YearlyCount `json:"node_counts_by_year"`
	ChartData  *ChartData `json:"chart_data,omitempty"`
}

// TechnologyEvolution tracks technology changes over time
type TechnologyEvolution struct {
	Technology string        `json:"technology"` // "V.32", "V.34", "ISDN", "Internet", etc.
	Timeline   []TrendPoint  `json:"timeline"`
	ChartData  *ChartData    `json:"chart_data,omitempty"`
}