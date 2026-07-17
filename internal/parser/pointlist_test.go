package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// writeTestPointlist writes content to a temp file with the given name and
// returns its path.
func writeTestPointlist(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}

// Excerpt shaped after the real r24/2024/boss/R24PNT.005 (boss format, FTS-5002 §2.1)
const bossFormatSample = `;A Fidonet R24 pointlist for Friday 05-Jan-2024 -- Day number 005 : 01815
;S Created by PLMAKECL 1.45 16.06.2021
;A
Boss,2:240/2199
,1,Kruemel_Boks!_Outpost,Altenholz,Christian_von_Busse,49-431-3292929,9600
,3,Kruemel_Boks!_Outpost,Altenholz,Gebhard_von_Busse,49-431-3292929,9600
Boss,2:240/5824
Pvt,1,Imzadi.1,Karlsruhe,Anna_Christina_Nass,-Unpublished-,300
Boss,2:240/5832
,1,DatenBahn_BBS_P1,Hamburg,Torsten_Bamberg,49-40-98670307,300
,1,DatenBahn_BBS_Dup,Hamburg,Torsten_Bamberg,49-40-98670307,300
`

// Excerpt shaped after the real r46/2024/poss/pnt46reg.012 (poss format, §2.3)
// with a deliberately stale header (November 2023 in a day-012 file of 2024).
const possFormatSample = `;A Region 46 Pointlist for Friday, November 24, 2023
;
Boss,2:4600/140
Point,1,Stellar_Way,Sebastopol_Crimea,Eugene_Glotov,-Unpublished-,300,CM,MO,XA,IBN
Point,2,Eagle's_Nest,Sebastopol_Crimea,Beliy_Orel,-Unpublished-,300,CM,MO,XA,IBN
;
Boss,2:463/68
Point,10,fido.net.ua,Kiev,Petro_Vlasenko,-Unpublished-,300,IBN
`

// Excerpt shaped after the real z2/2024/boss/Z2PNT.201: Boss lines carry full
// nodelist-style extras that must be tolerated and ignored.
const z2ExtendedBossSample = `;A FidoNet Zone 2 pointlist for Friday 19-Jul-2024 -- Day number 201
Boss,2:280/1049,The_Coast,Ouddorp_NL,Simon_Voortman,-Unpublished-,300,MO,INA:thecoastbbs.nl,IBN,CM
,1,Point_One,Ouddorp,Some_User,-Unpublished-,300
Boss,2:280/1208,UniCorn_BBS,Arnhem,Henri_Derksen,-Unpublished-,300,CM,MO,IBN:77.174.89.46,U,ENC
,2,Point_Two,Arnhem,Other_User,-Unpublished-,300,IBN:example.org
`

func TestPointlistParser_BossFormat(t *testing.T) {
	path := writeTestPointlist(t, "R24PNT.005", bossFormatSample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.SourcePriority = SourcePriorityRegional
	pp.Year = 2024
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.SourceFormat != PointlistFormatBoss {
		t.Errorf("expected format boss, got %s", result.SourceFormat)
	}
	if result.BossCount != 3 {
		t.Errorf("expected 3 bosses, got %d", result.BossCount)
	}
	if len(result.Points) != 5 {
		t.Fatalf("expected 5 points, got %d", len(result.Points))
	}

	first := result.Points[0]
	if first.Zone != 2 || first.Net != 240 || first.Node != 2199 || first.PointNum != 1 {
		t.Errorf("unexpected first point address: %d:%d/%d.%d", first.Zone, first.Net, first.Node, first.PointNum)
	}
	if first.SystemName != "Kruemel_Boks!_Outpost" || first.SysopName != "Christian_von_Busse" {
		t.Errorf("unexpected first point identity: %q / %q", first.SystemName, first.SysopName)
	}
	if first.MaxSpeed != 9600 {
		t.Errorf("expected max speed 9600, got %d", first.MaxSpeed)
	}
	if first.Domain != "fidonet" {
		t.Errorf("expected default domain fidonet, got %q", first.Domain)
	}
	if first.ListSource != "r24" || first.SourcePriority != SourcePriorityRegional {
		t.Errorf("unexpected source stamp: %s/%d", first.ListSource, first.SourcePriority)
	}

	// Pvt-flavoured line inside a boss file parses like any point line
	pvt := result.Points[2]
	if pvt.Net != 240 || pvt.Node != 5824 || pvt.PointNum != 1 || pvt.SystemName != "Imzadi.1" {
		t.Errorf("Pvt point line mishandled: %+v", pvt)
	}

	// Intra-file duplicate 2:240/5832.1 gets conflict tracking
	dup1, dup2 := result.Points[3], result.Points[4]
	if dup1.ConflictSequence != 0 || !dup1.HasConflict {
		t.Errorf("first duplicate should be seq 0 with conflict flag: %+v", dup1)
	}
	if dup2.ConflictSequence != 1 || !dup2.HasConflict {
		t.Errorf("second duplicate should be seq 1 with conflict flag: %+v", dup2)
	}

	// Date from filename day + Year
	wantDate := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	if !result.PointlistDate.Equal(wantDate) || result.DayNumber != 5 {
		t.Errorf("expected date %s day 5, got %s day %d", wantDate, result.PointlistDate, result.DayNumber)
	}

	// FTS id shape: z:n/n.p@date#seq
	if first.FtsId != "2:240/2199.1@2024-01-05#0" {
		t.Errorf("unexpected fts_id: %q", first.FtsId)
	}
}

func TestPointlistParser_PossFormat(t *testing.T) {
	path := writeTestPointlist(t, "pnt46reg.012", possFormatSample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r46"
	pp.SourcePriority = SourcePriorityRegional
	pp.Year = 2024
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.SourceFormat != PointlistFormatPoss {
		t.Errorf("expected format poss, got %s", result.SourceFormat)
	}
	if len(result.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(result.Points))
	}

	p := result.Points[0]
	if p.Zone != 2 || p.Net != 4600 || p.Node != 140 || p.PointNum != 1 {
		t.Errorf("unexpected point address: %d:%d/%d.%d", p.Zone, p.Net, p.Node, p.PointNum)
	}
	if !p.IsCM || !p.IsMO {
		t.Errorf("CM/MO flags not computed: %+v", p)
	}
	if !p.HasInet {
		t.Errorf("IBN flag should set has_inet")
	}

	// Filename day (012) must beat the stale header (2023-11-24) and produce a warning
	wantDate := time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC)
	if !result.PointlistDate.Equal(wantDate) {
		t.Errorf("filename day must beat stale header: got %s", result.PointlistDate)
	}
}

func TestPointlistParser_Z2ExtendedBossLines(t *testing.T) {
	path := writeTestPointlist(t, "Z2PNT.201", z2ExtendedBossSample)

	pp := NewPointlistParser(false)
	pp.ListSource = "z2"
	pp.SourcePriority = SourcePriorityZone
	pp.Year = 2024
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.BossCount != 2 {
		t.Errorf("expected 2 bosses (extras tolerated), got %d", result.BossCount)
	}
	if len(result.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(result.Points))
	}
	if result.SkippedLines != 0 {
		t.Errorf("extended Boss lines must not count as skipped, got %d skips", result.SkippedLines)
	}

	p2 := result.Points[1]
	if p2.Net != 280 || p2.Node != 1208 || p2.PointNum != 2 {
		t.Errorf("boss context from extended Boss line wrong: %+v", p2)
	}
	if !p2.HasInet {
		t.Errorf("IBN:example.org should set has_inet")
	}
	// z2 published day 201 of 2024 = Friday 19 July
	if result.PointlistDate.Weekday() != time.Friday {
		t.Errorf("expected Friday, got %s", result.PointlistDate.Weekday())
	}
}

func TestPointlistParser_DefaultZone(t *testing.T) {
	sample := "Boss,240/2188\n,1,Sys,Loc,Sysop,-Unpublished-,300\n"
	path := writeTestPointlist(t, "R24PNT.005", sample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.Year = 2024
	pp.DefaultZone = 2
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(result.Points) != 1 || result.Points[0].Zone != 2 {
		t.Fatalf("zoneless boss address must use DefaultZone: %+v", result.Points)
	}
}

func TestPointlistParser_CP866(t *testing.T) {
	utf8Content := "Boss,2:5020/545\n,1,Станция,Москва,Иван_Петров,-Unpublished-,300\n"
	encoded, err := charmap.CodePage866.NewEncoder().String(utf8Content)
	if err != nil {
		t.Fatalf("failed to encode test content: %v", err)
	}
	path := writeTestPointlist(t, "R50PNT.005", encoded)

	pp := NewPointlistParser(false)
	pp.ListSource = "r50"
	pp.Year = 2024
	pp.Charset = "cp866"
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(result.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result.Points))
	}
	p := result.Points[0]
	if p.SystemName != "Станция" || p.Location != "Москва" || p.SysopName != "Иван_Петров" {
		t.Errorf("CP866 decoding failed: %q %q %q", p.SystemName, p.Location, p.SysopName)
	}
	if !strings.Contains(p.RawLine, "Москва") {
		t.Errorf("raw_line must store decoded UTF-8: %q", p.RawLine)
	}
}

func TestPointlistParser_DownHoldPointLines(t *testing.T) {
	// Down and Hold are real FTS-5000 keyword values on point lines in the
	// corpus (622 Down / 758 Hold lines in a 310-archive sample) — they must
	// be imported, not dropped as "unknown line type".
	sample := `Boss,2:240/1
,1,Sys,Loc,Sysop,-Unpublished-,300
Down,2,DownSys,Loc,Sysop,-Unpublished-,300
Hold,3,HoldSys,Loc,Sysop,-Unpublished-,300
HOLD,4,HoldUpper,Loc,Sysop,-Unpublished-,300
,5,Sys5,Loc,Sysop,-Unpublished-,300
`
	path := writeTestPointlist(t, "R24PNT.005", sample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.Year = 2024
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(result.Points) != 5 {
		t.Fatalf("expected 5 points (Down/Hold included), got %d", len(result.Points))
	}
	if result.SkippedLines != 0 {
		t.Errorf("Down/Hold lines must not be skipped, got %d skips", result.SkippedLines)
	}
	if result.Points[1].SystemName != "DownSys" || result.Points[1].PointNum != 2 {
		t.Errorf("Down point line mishandled: %+v", result.Points[1])
	}
}

func TestPointlistParser_BossAddressWithPointSuffix(t *testing.T) {
	// Real corpus (r46/1997 PNT46REG.318) writes "Boss,2:465/101.0" — the
	// 4D .0 suffix must be tolerated or the whole block's points are orphaned.
	sample := `Boss,2:465/101.0
Point,1,Sys,Loc,Sysop,-Unpublished-,300
Point,2,Sys2,Loc,Sysop,-Unpublished-,300
`
	path := writeTestPointlist(t, "pnt46reg.318", sample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r46"
	pp.Year = 1997
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if result.BossCount != 1 || result.SkippedBosses != 0 {
		t.Fatalf("boss with .0 suffix rejected: bosses=%d skipped=%d", result.BossCount, result.SkippedBosses)
	}
	if len(result.Points) != 2 || result.Points[0].Net != 465 || result.Points[0].Node != 101 {
		t.Fatalf("points under .0-suffixed boss wrong: %+v", result.Points)
	}
}

func TestPointlistParser_DayOverflowUsesPreviousYear(t *testing.T) {
	// Real corpus: P28-LIST.366 (Friday 31-Dec-2004, leap day 366) is
	// archived under r28/2005/, so the script passes -year 2005. Day 366
	// does not exist in 2005 — the date must resolve to 2004-12-31, not
	// roll into 2006-01-01.
	sample := ";S Day number 366\nBoss,2:280/100\n,1,Sys,Loc,Sysop,-Unpublished-,300\n"
	path := writeTestPointlist(t, "P28-LIST.366", sample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r28"
	pp.Year = 2005
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	wantDate := time.Date(2004, 12, 31, 0, 0, 0, 0, time.UTC)
	if !result.PointlistDate.Equal(wantDate) {
		t.Fatalf("day 366 with -year 2005 must resolve to 2004-12-31, got %s", result.PointlistDate.Format("2006-01-02"))
	}
	warned := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "does not exist in 2005") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a year-correction warning, got %v", result.Warnings)
	}
}

func TestPointlistParser_HeaderDateFallback(t *testing.T) {
	// No 3-digit filename day: header must supply the date
	sample := ";A Fidonet R24 pointlist for Friday 05-Jan-2024 -- Day number 005\nBoss,2:240/1\n,1,Sys,Loc,Sysop,-Unpublished-,300\n"
	path := writeTestPointlist(t, "R24PNT.LST", sample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.Year = 2024
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if result.DayNumber != 5 {
		t.Errorf("expected header day 5, got %d", result.DayNumber)
	}
	wantDate := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	if !result.PointlistDate.Equal(wantDate) {
		t.Errorf("expected %s, got %s", wantDate, result.PointlistDate)
	}
}

func TestPointlistParser_NonConformingLines(t *testing.T) {
	sample := `Boss,2:240/1
,1,Sys,Loc,Sysop,-Unpublished-,300
Garbage line without commas enough
,notanumber,Sys,Loc,Sysop,-Unpublished-,300
,2,Sys2,Loc2,Sysop2,-Unpublished-,300
`
	path := writeTestPointlist(t, "R24PNT.005", sample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.Year = 2024
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("non-conforming lines must not be fatal: %v", err)
	}
	if len(result.Points) != 2 {
		t.Errorf("expected 2 valid points, got %d", len(result.Points))
	}
	if result.SkippedLines != 2 {
		t.Errorf("expected 2 skipped lines, got %d", result.SkippedLines)
	}
}

func TestDetectPointlistFormat(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{"boss", bossFormatSample, PointlistFormatBoss, false},
		{"poss", possFormatSample, PointlistFormatPoss, false},
		{"z2 extended boss", z2ExtendedBossSample, PointlistFormatBoss, false},
		{"pvt only", "Boss,2:240/1\nPvt,1,Sys,Loc,Sysop,-Unpublished-,300\n", PointlistFormatPvt, false},
		{"combined point format", "Zone,2,Europe,Somewhere,Coord,-Unpublished-,300\nHost,240,Net,Loc,Coord,-Unpublished-,300\n,2188,Sys,Loc,Sysop,49-1-2,9600\nPoint,1,PSys,PLoc,PSysop,-Unpublished-,300\n", PointlistFormatPoint, false},
		{"fakenet", "Host,10000,240/2188,Loc,Coord,-Unpublished-,300\n,1,Sys,Loc,Sysop,-Unpublished-,300\n", PointlistFormatFakenet, false},
		{"empty", ";A only comments\n", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectPointlistFormat(strings.Split(tc.content, "\n"))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got format %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

// Excerpt shaped after the real PTS24V7.LST (combined/V7 format, §2.2):
// nodelist-shaped context lines, points keyed "Point", no Zone line.
const combinedFormatSample = `;A Point Nodelist for Friday, January 3, 2025 -- Day number 003 : 62887
Region,24,Germany,RC_Germany,Torsten_Bamberg,49-40-98670308,9600,CM,XA,IBN,INA:bamberg-ot.de
Host,240,Host_Nordnetz,Hamburg,Torsten_Bamberg,49-40-98670308,9600,CM,XA,IBN,INA:datenbahn.dd-dns.de
,2188,Kruemel_Boks!,Boeblingen,Christian_von_Busse,-Unpublished-,300,CM,XA,IBN,INA:fido.kruemel.org
Point,1,Kruemel_Boks!,Boeblingen,Christian_von_Busse,-Unpublished-,300,
Point,6,Kruemel_Boks!_P12,MA,Manfred_Lang,-Unpublished-,300,
,2199,Kruemel_Boks!_North,Altenholz,Christian_von_Busse,49-431-3292929,9600,CM,XA
Point,1,Kruemel_Boks!_Outpost,Altenholz,Christian_von_Busse,49-431-3292929,9600,
Host,246,Some_Other_Net,Somewhere,Armin_Kleinmann,-Unpublished-,300
Hub,100,A_Hub,Somewhere,Some_Hub,-Unpublished-,300
Point,5,Hub_Point,Somewhere,Hub_Pointop,-Unpublished-,300,CM
`

func TestPointlistParser_CombinedFormat(t *testing.T) {
	path := writeTestPointlist(t, "R29PNT_P.003", combinedFormatSample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.SourcePriority = SourcePriorityRegional
	pp.Year = 2025
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.SourceFormat != PointlistFormatPoint {
		t.Errorf("expected format point, got %s", result.SourceFormat)
	}
	if len(result.Points) != 4 {
		t.Fatalf("expected 4 points, got %d: %+v", len(result.Points), result.Points)
	}
	// 3 distinct bosses emitted points: 240/2188, 240/2199, 246/100
	if result.BossCount != 3 {
		t.Errorf("expected 3 bosses with points, got %d", result.BossCount)
	}

	// V7 lists have no Zone line: DefaultZone (2) applies
	first := result.Points[0]
	if first.Zone != 2 || first.Net != 240 || first.Node != 2188 || first.PointNum != 1 {
		t.Errorf("unexpected first point address: %d:%d/%d.%d", first.Zone, first.Net, first.Node, first.PointNum)
	}

	// Node context switches to 2199 for the third point
	third := result.Points[2]
	if third.Net != 240 || third.Node != 2199 || third.PointNum != 1 {
		t.Errorf("node context not tracked: %d:%d/%d.%d", third.Zone, third.Net, third.Node, third.PointNum)
	}

	// Hub line under Host,246 is context: point 5 belongs to 2:246/100
	hub := result.Points[3]
	if hub.Net != 246 || hub.Node != 100 || hub.PointNum != 5 || !hub.IsCM {
		t.Errorf("Hub context mishandled: %+v", hub)
	}

	// Node lines are context only, never imported
	for _, p := range result.Points {
		if p.PointNum == 2188 || p.PointNum == 2199 {
			t.Errorf("node line imported as point: %+v", p)
		}
	}
}

// Excerpt shaped after the real POINTS24.003 (fakenet, sysname convention)
// with a DK-POINT-style UBOSS block and an unresolvable block appended.
const fakenetFormatSample = `;A Zone Nodelist for Friday, April 3, 1992 -- Day number 094 : xxxxx
Zone,2,Europe_etc,Finland,Ron_Dwight,358-0-2983308,9600,CM,MO,XA
Region,10000,NPK_240,Bad_Ueberkingen,Ulrich_Schroeter,49-7331-9861042,9600,CM,XA,MO
Host,10251,240/2188,Boeblingen,Christian_von_Busse,000-000-000-000,300,CM,XA,IBN,INA:fido.kruemel.org
,1,Kruemel_Boks!,Boeblingen,Christian_von_Busse,000-000-000-000,300
,6,Kruemel_Boks!_P12,MA,Manfred_Lang,000-000-000-000,300
Host,10026,Computerland_BBS,Ballerup,Gorm_Joergensen,45-42-971621,9600,CM,V22,XA,UBOSS:230/26
,1,Gorm's_point,Bagsvaerd,Gorm_Joergensen,-Unpublished-,9600,MO
Host,10099,No_Boss_Here,Nowhere,Some_Sysop,-Unpublished-,300,CM
,1,Orphan_Point,Nowhere,Someone,-Unpublished-,300
`

func TestPointlistParser_FakenetFormat(t *testing.T) {
	path := writeTestPointlist(t, "POINTS24.094", fakenetFormatSample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.SourcePriority = SourcePriorityRegional
	pp.Year = 1992
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.SourceFormat != PointlistFormatFakenet {
		t.Errorf("expected format fakenet, got %s", result.SourceFormat)
	}
	if len(result.Points) != 3 {
		t.Fatalf("expected 3 points, got %d: %+v", len(result.Points), result.Points)
	}
	if result.BossCount != 2 {
		t.Errorf("expected 2 resolved bosses, got %d", result.BossCount)
	}
	if result.SkippedBosses != 1 {
		t.Errorf("expected 1 skipped boss block, got %d", result.SkippedBosses)
	}

	// Sysname convention: Host,10251,240/2188 → boss 2:240/2188 (Zone line zone)
	first := result.Points[0]
	if first.Zone != 2 || first.Net != 240 || first.Node != 2188 || first.PointNum != 1 {
		t.Errorf("sysname boss resolution failed: %d:%d/%d.%d", first.Zone, first.Net, first.Node, first.PointNum)
	}

	// UBOSS convention: UBOSS:230/26 → boss 2:230/26
	uboss := result.Points[2]
	if uboss.Zone != 2 || uboss.Net != 230 || uboss.Node != 26 || uboss.PointNum != 1 {
		t.Errorf("UBOSS boss resolution failed: %d:%d/%d.%d", uboss.Zone, uboss.Net, uboss.Node, uboss.PointNum)
	}
	if uboss.SysopName != "Gorm_Joergensen" || !uboss.IsMO {
		t.Errorf("UBOSS point fields mishandled: %+v", uboss)
	}

	// The unresolvable Host block's leaf lines are skipped, not misattached
	for _, p := range result.Points {
		if p.SystemName == "Orphan_Point" {
			t.Errorf("orphan point imported despite unresolvable Host block: %+v", p)
		}
	}
}

// Nodelist with inline Point lines (used by some FTN networks): points are
// collected only with CollectPoints and belong to the preceding node line.
const inlinePointNodelistSample = `;A Test nodelist for Friday, January 5, 2024 -- Day number 005 : 00000
Zone,2,Gateway,Somewhere,Coord,-Unpublished-,300,CM
Host,5001,Some_Net,Somewhere,Host_Op,-Unpublished-,300,CM
,100,Test_System,Somewhere,Test_Sysop,-Unpublished-,300,CM,IBN
Point,1,Point_One,Somewhere,Point_Op,-Unpublished-,300,MO
Point,2,Point_Two,Elsewhere,Other_Op,-Unpublished-,300
,101,Next_System,Somewhere,Next_Sysop,-Unpublished-,300
`

func TestParser_InlinePointCollection(t *testing.T) {
	path := writeTestPointlist(t, "nodelist.005", inlinePointNodelistSample)

	// Default: Point lines are dropped, no behaviour change
	p := New(false)
	result, err := p.ParseFileWithCRC(path)
	if err != nil {
		t.Fatalf("ParseFileWithCRC failed: %v", err)
	}
	if len(result.Points) != 0 {
		t.Errorf("expected no points without CollectPoints, got %d", len(result.Points))
	}
	if len(result.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(result.Nodes))
	}

	// With CollectPoints: points attach to the preceding node
	p = New(false)
	p.CollectPoints = true
	p.SetDomain("testnet")
	result, err = p.ParseFileWithCRC(path)
	if err != nil {
		t.Fatalf("ParseFileWithCRC failed: %v", err)
	}
	if len(result.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(result.Nodes))
	}
	if len(result.Points) != 2 {
		t.Fatalf("expected 2 inline points, got %d", len(result.Points))
	}

	first := result.Points[0]
	if first.Zone != 2 || first.Net != 5001 || first.Node != 100 || first.PointNum != 1 {
		t.Errorf("unexpected point address: %d:%d/%d.%d", first.Zone, first.Net, first.Node, first.PointNum)
	}
	if first.SystemName != "Point_One" || !first.IsMO {
		t.Errorf("point fields mishandled: %+v", first)
	}
	if first.Domain != "testnet" {
		t.Errorf("expected domain testnet, got %q", first.Domain)
	}
	if first.ListSource != NodelistPointSource || first.SourcePriority != SourcePriorityNet || first.SourceFormat != NodelistPointSource {
		t.Errorf("unexpected source stamp: %s/%d/%s", first.ListSource, first.SourcePriority, first.SourceFormat)
	}
	if first.PointlistDate.IsZero() || first.DayNumber != 5 {
		t.Errorf("point not stamped with nodelist date: %s day %d", first.PointlistDate, first.DayNumber)
	}
}

// Excerpt shaped after the real POINTS24.342 of 1989: descriptive Host, boss
// addresses on Hub lines, and full 4-D addresses in the leaf sysname field.
const fakenetHubFormatSample = `;A Point Nodelist for Friday, December 8, 1989 -- Day number 342 : xxxxx
Host,24000,PointNet_Region_24,FRG_(Germany),_confidental_,49-203-408799,300,CM,XA,MO
Hub,1100,241/1,D-4630_Bochum,Mario_Remfeld,49-2327-320077,9600,CM,XA,HST,V32
,1101,241/1.1101,D-4300_Essen,Volker_Zinke,49-2327-320077,9600,CM,XA,HST,V32
,1102,241/1.1102,D-4630_Bochum,Oliver_Lohkamp,49-2327-320077,9600,CM,XA,HST,V32
Hub,1200,Plain_Grouping_Hub,Nowhere,Nobody,-Unpublished-,300
,1201,242/7.5,D-4000_Duesseldorf,Some_One,-Unpublished-,2400,MO
,1202,Not_An_Address,Nowhere,No_Boss,-Unpublished-,2400
`

func TestPointlistParser_FakenetHubFormat(t *testing.T) {
	path := writeTestPointlist(t, "POINTS24.342", fakenetHubFormatSample)

	pp := NewPointlistParser(false)
	pp.ListSource = "r24"
	pp.SourcePriority = SourcePriorityRegional
	pp.Year = 1989
	result, err := pp.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.SourceFormat != PointlistFormatFakenet {
		t.Errorf("expected format fakenet, got %s", result.SourceFormat)
	}
	if len(result.Points) != 3 {
		t.Fatalf("expected 3 points, got %d: %+v", len(result.Points), result.Points)
	}

	// Hub-sysname convention: Hub,1100,241/1 → boss 2:241/1
	first := result.Points[0]
	if first.Zone != 2 || first.Net != 241 || first.Node != 1 || first.PointNum != 1101 {
		t.Errorf("Hub boss resolution failed: %d:%d/%d.%d", first.Zone, first.Net, first.Node, first.PointNum)
	}

	// Leaf-4D-sysname convention rescues leaves under a plain grouping Hub
	rescued := result.Points[2]
	if rescued.Zone != 2 || rescued.Net != 242 || rescued.Node != 7 || rescued.PointNum != 5 {
		t.Errorf("leaf 4-D sysname resolution failed: %d:%d/%d.%d", rescued.Zone, rescued.Net, rescued.Node, rescued.PointNum)
	}

	// A leaf with neither context boss nor 4-D sysname is skipped
	for _, p := range result.Points {
		if p.SystemName == "Not_An_Address" {
			t.Errorf("boss-less leaf imported: %+v", p)
		}
	}
}

func TestDerivePointlistSource(t *testing.T) {
	cases := []struct {
		filename     string
		wantSource   string
		wantPriority uint8
		wantOK       bool
	}{
		{"R24PNT.005", "r24", SourcePriorityRegional, true},
		{"r24pnt.005.gz", "r24", SourcePriorityRegional, true},
		{"Z2PNT.201", "z2", SourcePriorityZone, true},
		{"R50PNT.293", "r50", SourcePriorityRegional, true}, // stray in R56/BOSS: filename wins
		{"R56PNT.202", "r56", SourcePriorityRegional, true},
		{"pnt46reg.012", "r46", SourcePriorityRegional, true},
		{"P28-LIST.Z00", "r28", SourcePriorityRegional, true},
		{"R45POINT.Z10", "r45", SourcePriorityRegional, true},
		{"POINT_48.203", "r48", SourcePriorityRegional, true},
		{"POINTR34.101", "r34", SourcePriorityRegional, true},
		{"PTLSTR34.101", "r34", SourcePriorityRegional, true},
		{"R29PNT_B.005", "r29", SourcePriorityRegional, true},
		{"R29PNT_P.005", "r29", SourcePriorityRegional, true}, // combined/V7 flavour, same source
		// Fakenet series share the region's list_source: the gate gap-fills
		{"POINTS24.201", "r24", SourcePriorityRegional, true},
		{"DK-POINT.094", "r23", SourcePriorityRegional, true},
		{"DK-POINT.PVT", "r23", SourcePriorityRegional, true},
		// Diffs and unknown families must be rejected — including .D##
		// diffs extracted from a series' own archives
		{"R24PNT_D.201", "", 0, false},
		{"PR24DIFF.005", "", 0, false},
		{"DKP-DIFF.005", "", 0, false},
		{"POINTS24.D02", "", 0, false},
		{"files.bbs", "", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			source, priority, ok := DerivePointlistSource(tc.filename)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if source != tc.wantSource || priority != tc.wantPriority {
				t.Errorf("got (%s, %d), want (%s, %d)", source, priority, tc.wantSource, tc.wantPriority)
			}
		})
	}
}

// TestPointlistParser_RealCorpus parses full real files when the corpus is
// present on this machine; skipped elsewhere.
func TestPointlistParser_RealCorpus(t *testing.T) {
	corpus := "/home/dp/FIDO/pointlist/pointlists-export/fidohist-pntlist"
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("pointlist corpus not available")
	}
	if testing.Short() {
		t.Skip("skipping corpus test in short mode")
	}

	cases := []struct {
		file      string
		year      int
		charset   string
		minPoints int
	}{
		{"r46/2024/poss/pnt46reg.012", 2024, "cp866", 30},
	}

	// PNT46REG.318 of 1997 (extracted) has Boss,...0-suffixed addresses; the
	// peer review measured 15 skipped bosses / 156 orphaned points before the
	// fix. Covered by TestPointlistParser_BossAddressWithPointSuffix for CI;
	// the extracted file is checked here when present.

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(corpus, tc.file)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("sample %s not present", tc.file)
			}
			pp := NewPointlistParser(false)
			pp.Year = tc.year
			pp.Charset = tc.charset
			source, priority, ok := DerivePointlistSource(path)
			if !ok {
				t.Fatalf("cannot derive source for %s", path)
			}
			pp.ListSource = source
			pp.SourcePriority = priority

			result, err := pp.ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile failed: %v", err)
			}
			if len(result.Points) < tc.minPoints {
				t.Errorf("expected at least %d points, got %d", tc.minPoints, len(result.Points))
			}
			if result.PointlistDate.IsZero() {
				t.Error("date not resolved")
			}
			t.Logf("%s: %d points, %d bosses, %d skipped, date %s",
				tc.file, len(result.Points), result.BossCount, result.SkippedLines,
				result.PointlistDate.Format("2006-01-02"))
		})
	}
}
