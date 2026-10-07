package schema

import (
	"strconv"
	"strings"

	"github.com/standards-lab/sqlate/migrate"
)

var statusHeader = []string{"set", "table", "version", "latest", "pending", "dirty"}

// statusRows renders each set's status as one row under statusHeader. The
// pending column lists the pending migrations as "N name", or none.
func statusRows(sets []migrate.SetStatus) [][]string {
	rows := make([][]string, 0, len(sets))
	for _, s := range sets {
		pending := "none"
		if len(s.Pending) > 0 {
			names := make([]string, 0, len(s.Pending))
			for _, m := range s.Pending {
				names = append(names, strconv.Itoa(m.Version)+" "+m.Name)
			}
			pending = strings.Join(names, ", ")
		}
		rows = append(rows, []string{
			s.Name, s.Table, strconv.Itoa(s.Version), strconv.Itoa(s.Latest), pending, strconv.FormatBool(s.Dirty),
		})
	}
	return rows
}
