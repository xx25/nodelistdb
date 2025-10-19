package emsi

import (
	"regexp"
	"strings"

	"github.com/nodelistdb/internal/testing/logging"
)

// BannerSoftware represents software info extracted from banner text
type BannerSoftware struct {
	Name     string
	Version  string
	Platform string
	Source   string // Always "banner" for this type
}

// bannerPattern represents a regex pattern and extraction function for banner parsing
type bannerPattern struct {
	name    string
	pattern *regexp.Regexp
	extract func([]string) *BannerSoftware
}

// bannerPatterns contains all known banner text patterns for mailer software
var bannerPatterns = []bannerPattern{
	// Pattern 1: "binkleyforce 0.22.9/archlinux (c) 1997-2000 by Alexander Belkin"
	{
		name:    "binkleyforce",
		pattern: regexp.MustCompile(`(?i)binkleyforce\s+([\d.]+)/([\w]+)`),
		extract: func(matches []string) *BannerSoftware {
			return &BannerSoftware{
				Name:     "binkleyforce",
				Version:  matches[1],
				Platform: matches[2],
				Source:   "banner",
			}
		},
	},

	// Pattern 2: "using qico v0.57.1xe [2007-03-20 01:58]"
	// or "using qico-m19 v0.59 [2015-05-14 14:48]"
	{
		name:    "qico",
		pattern: regexp.MustCompile(`using\s+(qico[\w-]*)\s+v([\d.]+[\w]*)`),
		extract: func(matches []string) *BannerSoftware {
			return &BannerSoftware{
				Name:    matches[1],
				Version: matches[2],
				Source:  "banner",
			}
		},
	},

	// Pattern 3: "ifcico 2.14tx8 [Linux/x86]"
	{
		name:    "ifcico",
		pattern: regexp.MustCompile(`(?i)ifcico\s+([\d.]+[\w]*)\s*\[([^\]]+)\]`),
		extract: func(matches []string) *BannerSoftware {
			return &BannerSoftware{
				Name:     "ifcico",
				Version:  matches[1],
				Platform: matches[2],
				Source:   "banner",
			}
		},
	},

	// Pattern 4: Generic "software version/platform (c) ..."
	{
		name:    "generic_slash",
		pattern: regexp.MustCompile(`([a-zA-Z][\w-]+)\s+([\d.]+[\w]*)[/\s]+([\w]+)\s+\(c\)`),
		extract: func(matches []string) *BannerSoftware {
			return &BannerSoftware{
				Name:     matches[1],
				Version:  matches[2],
				Platform: matches[3],
				Source:   "banner",
			}
		},
	},

	// Pattern 5: Generic "software version"
	{
		name:    "generic_version",
		pattern: regexp.MustCompile(`([a-zA-Z][\w-]+)\s+([\d]+\.[\d.]+[\w]*)`),
		extract: func(matches []string) *BannerSoftware {
			return &BannerSoftware{
				Name:    matches[1],
				Version: matches[2],
				Source:  "banner",
			}
		},
	},
}

// extractSoftwareFromBanner attempts to parse software info from banner text
func (s *Session) extractSoftwareFromBanner() *BannerSoftware {
	if s.bannerText == "" {
		return nil
	}

	// Clean up banner text
	banner := s.bannerText

	// Remove EMSI sequences
	banner = strings.ReplaceAll(banner, "**EMSI_REQA77E", "")
	banner = strings.ReplaceAll(banner, "**EMSI_REQ", "")
	banner = strings.ReplaceAll(banner, "**EMSI_DAT", "")

	// Trim whitespace and leading characters
	banner = strings.TrimSpace(banner)
	banner = strings.Trim(banner, "\r\n+* ")

	if banner == "" {
		return nil
	}

	if s.debug {
		logging.Debugf("EMSI: Attempting to extract software from banner: %q", banner)
	}

	// Try each pattern in order
	for _, bp := range bannerPatterns {
		if matches := bp.pattern.FindStringSubmatch(banner); matches != nil {
			software := bp.extract(matches)
			if s.debug {
				logging.Debugf("EMSI: Extracted software from banner (pattern=%s): %s %s %s",
					bp.name, software.Name, software.Version, software.Platform)
			}
			return software
		}
	}

	if s.debug {
		logging.Debugf("EMSI: Could not extract software from banner using known patterns")
	}
	return nil
}
