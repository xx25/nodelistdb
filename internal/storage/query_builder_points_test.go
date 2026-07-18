package storage

import (
	"strings"
	"testing"

	"github.com/nodelistdb/internal/database"
)

// The two-phase lifetime SQL repeats the WHERE clause, so
// SearchPointsWithLifetime binds its WHERE args twice plus limit, offset,
// limit. This guards the placeholder-count invariant between the generated
// SQL and that arg-assembly convention — editing one without the other would
// only fail at query time against a live ClickHouse.
func TestSearchPointsLifetimeSQLBindCounts(t *testing.T) {
	qb := NewQueryBuilder()
	intp := func(v int) *int { return &v }
	strp := func(v string) *string { return &v }

	filters := map[string]database.PointFilter{
		"empty":         {},
		"zone only":     {Zone: intp(2)},
		"full identity": {Zone: intp(2), Net: intp(5001), Node: intp(100), PointNum: intp(1), ListSource: strp("pointlist")},
		"text only":     {SystemName: strp("bbs"), Location: strp("moscow"), SysopName: strp("john_doe")},
		"everything":    {Zone: intp(2), Net: intp(5001), SystemName: strp("bbs"), SysopName: strp("doe")},
	}

	for name, filter := range filters {
		t.Run(name, func(t *testing.T) {
			identityWhere, identityArgs, attrWhere, attrArgs := qb.BuildPointFilterConditions(filter)
			where := identityWhere
			if attrWhere != "" {
				if where != "" {
					where += " AND " + attrWhere
				} else {
					where = attrWhere
				}
			}
			sql := qb.SearchPointsLifetimeSQL(where)

			// Mirrors the assembly in SearchPointsWithLifetime: the WHERE
			// binds (domain, domain, identity, attr) twice, then limit,
			// offset (inner), limit (outer).
			whereBinds := 2 + len(identityArgs) + len(attrArgs)
			wantArgs := 2*whereBinds + 3

			if got := strings.Count(sql, "?"); got != wantArgs {
				t.Errorf("placeholders = %d, want %d (WHERE binds %d)\nSQL: %s", got, wantArgs, whereBinds, sql)
			}
		})
	}
}
