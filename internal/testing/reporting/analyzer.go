package reporting

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// ProblemAnalyzer analyzes test results to detect problems
type ProblemAnalyzer struct {
	// Thresholds for problem detection
	failureThreshold      float64 // Percentage of failures to consider problematic
	responseTimeThreshold uint32  // Response time in ms to consider slow
	
	// Problem tracking
	problemNodes map[string]*ProblemNode
}

// ProblemNode represents a node with detected problems
type ProblemNode struct {
	Address          string
	Zone, Net, Node  int
	Problems         []Problem
	LastSeen         time.Time
	ConsecutiveFails int
	TotalTests       int
	TotalFailures    int
}

// Problem represents a specific issue detected
type Problem struct {
	Type        string    // dns_failure, protocol_failure, slow_response, etc.
	Protocol    string    // Which protocol if applicable
	Description string
	FirstSeen   time.Time
	LastSeen    time.Time
	Occurrences int
}

// ProblemReport represents a complete problem analysis report
type ProblemReport struct {
	GeneratedAt    time.Time
	TotalNodes     int
	ProblematicNodes int
	Problems       []ProblemSummary
	TopIssues      []IssueStatistic
	Recommendations []string
}

// ProblemSummary summarizes problems for a node
type ProblemSummary struct {
	Node     *ProblemNode
	Severity string // critical, warning, info
	Issues   []string
}

// IssueStatistic represents statistics for a type of issue
type IssueStatistic struct {
	IssueType    string
	Count        int
	Percentage   float64
	AffectedNodes []string
}

// NewProblemAnalyzer creates a new problem analyzer
func NewProblemAnalyzer() *ProblemAnalyzer {
	return &ProblemAnalyzer{
		failureThreshold:      0.5,  // 50% failure rate
		responseTimeThreshold: 5000, // 5 seconds
		problemNodes:          make(map[string]*ProblemNode),
	}
}

// AnalyzeResults analyzes test results to detect problems
func (a *ProblemAnalyzer) AnalyzeResults(results []*models.TestResult) *ProblemReport {
	report := &ProblemReport{
		GeneratedAt: time.Now(),
		TotalNodes:  len(results),
		Problems:    []ProblemSummary{},
		TopIssues:   []IssueStatistic{},
	}
	
	// Track issue types
	issueCount := make(map[string][]string)
	
	for _, result := range results {
		problems := a.detectProblems(result)
		if len(problems) > 0 {
			nodeKey := result.Address
			
			// Update or create problem node
			pNode, exists := a.problemNodes[nodeKey]
			if !exists {
				pNode = &ProblemNode{
					Address: result.Address,
					Zone:    result.Zone,
					Net:     result.Net,
					Node:    result.Node,
					Problems: []Problem{},
				}
				a.problemNodes[nodeKey] = pNode
			}
			
			// Update node statistics
			pNode.LastSeen = result.TestTime
			pNode.TotalTests++
			
			// Add new problems
			for _, p := range problems {
				a.addProblem(pNode, p)
				
				// Track for statistics
				if issueCount[p.Type] == nil {
					issueCount[p.Type] = []string{}
				}
				issueCount[p.Type] = append(issueCount[p.Type], nodeKey)
			}
			
			// Create problem summary
			severity := a.determineSeverity(pNode)
			issues := a.formatIssues(pNode)
			
			report.Problems = append(report.Problems, ProblemSummary{
				Node:     pNode,
				Severity: severity,
				Issues:   issues,
			})
		}
	}
	
	// Calculate statistics
	report.ProblematicNodes = len(report.Problems)
	
	// Generate top issues
	for issueType, nodes := range issueCount {
		report.TopIssues = append(report.TopIssues, IssueStatistic{
			IssueType:     issueType,
			Count:         len(nodes),
			Percentage:    float64(len(nodes)) / float64(report.TotalNodes) * 100,
			AffectedNodes: nodes,
		})
	}
	
	// Sort by count
	sort.Slice(report.TopIssues, func(i, j int) bool {
		return report.TopIssues[i].Count > report.TopIssues[j].Count
	})
	
	// Generate recommendations
	report.Recommendations = a.generateRecommendations(report)
	
	return report
}

// detectProblems detects problems in a test result
func (a *ProblemAnalyzer) detectProblems(result *models.TestResult) []Problem {
	problems := []Problem{}
	now := time.Now()
	
	// Check DNS issues
	if result.DNSError != "" {
		problems = append(problems, Problem{
			Type:        "dns_failure",
			Description: fmt.Sprintf("DNS resolution failed: %s", result.DNSError),
			FirstSeen:   now,
			LastSeen:    now,
			Occurrences: 1,
		})
	}
	
	// Check if no IPs were resolved but hostname exists
	if result.Hostname != "" && len(result.ResolvedIPv4) == 0 && len(result.ResolvedIPv6) == 0 {
		problems = append(problems, Problem{
			Type:        "no_ip_resolved",
			Description: "Hostname exists but no IP addresses resolved",
			FirstSeen:   now,
			LastSeen:    now,
			Occurrences: 1,
		})
	}
	
	// Check BinkP issues
	if result.BinkPResult != nil && result.BinkPResult.Tested {
		if !result.BinkPResult.Success {
			problems = append(problems, Problem{
				Type:        "protocol_failure",
				Protocol:    "BinkP",
				Description: fmt.Sprintf("BinkP connection failed: %s", result.BinkPResult.Error),
				FirstSeen:   now,
				LastSeen:    now,
				Occurrences: 1,
			})
		} else if result.BinkPResult.ResponseMs > a.responseTimeThreshold {
			problems = append(problems, Problem{
				Type:        "slow_response",
				Protocol:    "BinkP",
				Description: fmt.Sprintf("BinkP response time %dms exceeds threshold", result.BinkPResult.ResponseMs),
				FirstSeen:   now,
				LastSeen:    now,
				Occurrences: 1,
			})
		}
	}
	
	// Check IFCICO issues
	if result.IfcicoResult != nil && result.IfcicoResult.Tested {
		if !result.IfcicoResult.Success {
			problems = append(problems, Problem{
				Type:        "protocol_failure",
				Protocol:    "IFCICO",
				Description: fmt.Sprintf("IFCICO connection failed: %s", result.IfcicoResult.Error),
				FirstSeen:   now,
				LastSeen:    now,
				Occurrences: 1,
			})
		}
	}
	
	// Check if node is not operational at all
	if !result.IsOperational && len(result.ResolvedIPv4) > 0 {
		problems = append(problems, Problem{
			Type:        "node_unreachable",
			Description: "Node has IP but no protocols responding",
			FirstSeen:   now,
			LastSeen:    now,
			Occurrences: 1,
		})
	}
	
	// Check address validation
	if !result.AddressValidated && result.IsOperational {
		problems = append(problems, Problem{
			Type:        "address_mismatch",
			Description: "Node responding but address doesn't match nodelist",
			FirstSeen:   now,
			LastSeen:    now,
			Occurrences: 1,
		})
	}
	
	return problems
}

// addProblem adds or updates a problem for a node
func (a *ProblemAnalyzer) addProblem(node *ProblemNode, newProblem Problem) {
	// Check if problem already exists
	for i, p := range node.Problems {
		if p.Type == newProblem.Type && p.Protocol == newProblem.Protocol {
			// Update existing problem
			node.Problems[i].LastSeen = newProblem.LastSeen
			node.Problems[i].Occurrences++
			return
		}
	}
	
	// Add new problem
	node.Problems = append(node.Problems, newProblem)
}

// determineSeverity determines the severity of node problems
func (a *ProblemAnalyzer) determineSeverity(node *ProblemNode) string {
	// Critical: Node completely unreachable or DNS failure
	for _, p := range node.Problems {
		if p.Type == "dns_failure" || p.Type == "node_unreachable" {
			return "critical"
		}
	}
	
	// Warning: Multiple protocol failures or consistent failures
	protocolFailures := 0
	for _, p := range node.Problems {
		if p.Type == "protocol_failure" {
			protocolFailures++
		}
	}
	
	if protocolFailures >= 2 {
		return "warning"
	}
	
	// Warning: High failure rate
	if node.TotalTests > 0 {
		failureRate := float64(node.TotalFailures) / float64(node.TotalTests)
		if failureRate > a.failureThreshold {
			return "warning"
		}
	}
	
	return "info"
}

// formatIssues formats issues for display
func (a *ProblemAnalyzer) formatIssues(node *ProblemNode) []string {
	issues := []string{}
	
	for _, p := range node.Problems {
		issue := p.Type
		if p.Protocol != "" {
			issue = fmt.Sprintf("%s (%s)", p.Type, p.Protocol)
		}
		if p.Occurrences > 1 {
			issue = fmt.Sprintf("%s x%d", issue, p.Occurrences)
		}
		issues = append(issues, issue)
	}
	
	return issues
}

// generateRecommendations generates recommendations based on problems
func (a *ProblemAnalyzer) generateRecommendations(report *ProblemReport) []string {
	recommendations := []string{}
	
	// Check for widespread DNS issues
	dnsFailures := 0
	for _, issue := range report.TopIssues {
		if issue.IssueType == "dns_failure" {
			dnsFailures = issue.Count
			break
		}
	}
	
	if dnsFailures > 0 {
		percentage := float64(dnsFailures) / float64(report.TotalNodes) * 100
		if percentage > 10 {
			recommendations = append(recommendations,
				fmt.Sprintf("%.1f%% of nodes have DNS failures - check DNS resolver configuration", percentage))
		}
	}
	
	// Check for protocol-specific issues
	protocolIssues := make(map[string]int)
	for _, issue := range report.TopIssues {
		if issue.IssueType == "protocol_failure" {
			for _, summary := range report.Problems {
				for _, p := range summary.Node.Problems {
					if p.Type == "protocol_failure" && p.Protocol != "" {
						protocolIssues[p.Protocol]++
					}
				}
			}
		}
	}
	
	for protocol, count := range protocolIssues {
		if count > 5 {
			recommendations = append(recommendations,
				fmt.Sprintf("%d nodes have %s failures - investigate %s connectivity", count, protocol, protocol))
		}
	}
	
	// Check for slow responses
	slowNodes := 0
	for _, summary := range report.Problems {
		for _, p := range summary.Node.Problems {
			if p.Type == "slow_response" {
				slowNodes++
				break
			}
		}
	}
	
	if slowNodes > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("%d nodes have slow response times - consider adjusting timeout thresholds", slowNodes))
	}
	
	return recommendations
}

// GetProblemNodes returns all nodes with problems
func (a *ProblemAnalyzer) GetProblemNodes() []*ProblemNode {
	nodes := make([]*ProblemNode, 0, len(a.problemNodes))
	for _, node := range a.problemNodes {
		nodes = append(nodes, node)
	}
	
	// Sort by severity and address
	sort.Slice(nodes, func(i, j int) bool {
		sev1 := a.determineSeverity(nodes[i])
		sev2 := a.determineSeverity(nodes[j])
		
		if sev1 != sev2 {
			// Critical > Warning > Info
			sevOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
			return sevOrder[sev1] < sevOrder[sev2]
		}
		
		return nodes[i].Address < nodes[j].Address
	})
	
	return nodes
}

// FormatReport formats a problem report as text
func (a *ProblemAnalyzer) FormatReport(report *ProblemReport) string {
	var sb strings.Builder
	
	sb.WriteString("=== FidoNet Node Problem Analysis Report ===\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Total Nodes Tested: %d\n", report.TotalNodes))
	sb.WriteString(fmt.Sprintf("Problematic Nodes: %d (%.1f%%)\n\n",
		report.ProblematicNodes,
		float64(report.ProblematicNodes)/float64(report.TotalNodes)*100))
	
	// Top issues
	if len(report.TopIssues) > 0 {
		sb.WriteString("=== Top Issues ===\n")
		for i, issue := range report.TopIssues {
			if i >= 5 {
				break // Show top 5
			}
			sb.WriteString(fmt.Sprintf("%d. %s: %d nodes (%.1f%%)\n",
				i+1, issue.IssueType, issue.Count, issue.Percentage))
		}
		sb.WriteString("\n")
	}
	
	// Recommendations
	if len(report.Recommendations) > 0 {
		sb.WriteString("=== Recommendations ===\n")
		for _, rec := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("• %s\n", rec))
		}
		sb.WriteString("\n")
	}
	
	// Problem nodes by severity
	criticalNodes := []ProblemSummary{}
	warningNodes := []ProblemSummary{}

	for _, summary := range report.Problems {
		switch summary.Severity {
		case "critical":
			criticalNodes = append(criticalNodes, summary)
		case "warning":
			warningNodes = append(warningNodes, summary)
		}
	}
	
	// Critical nodes
	if len(criticalNodes) > 0 {
		sb.WriteString("=== Critical Issues ===\n")
		for _, summary := range criticalNodes {
			sb.WriteString(fmt.Sprintf("%s: %s\n",
				summary.Node.Address,
				strings.Join(summary.Issues, ", ")))
		}
		sb.WriteString("\n")
	}
	
	// Warning nodes
	if len(warningNodes) > 0 {
		sb.WriteString("=== Warning Issues ===\n")
		for _, summary := range warningNodes {
			if len(warningNodes) > 10 && len(sb.String()) > 5000 {
				sb.WriteString(fmt.Sprintf("... and %d more nodes with warnings\n",
					len(warningNodes)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("%s: %s\n",
				summary.Node.Address,
				strings.Join(summary.Issues, ", ")))
		}
		sb.WriteString("\n")
	}
	
	return sb.String()
}