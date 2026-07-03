package profile

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

type barTypeSpec struct {
	name  string
	color string // ANSI 256-color index
	get   func(HotspotRow) int
}

var barTypes = []barTypeSpec{
	{"R", "39", func(r HotspotRow) int { return r.Running }},
	{"L", "196", func(r HotspotRow) int { return r.Lock }},
	{"N", "226", func(r HotspotRow) int { return r.NetIO }},
	{"D", "46", func(r HotspotRow) int { return r.DiskIO }},
	{"W", "244", func(r HotspotRow) int { return r.Wait }},
}

const miniBarWidth = 8
const nameColWidth = 38

// ToBars renders per-type horizontal mini-bars for each hotspot method.
func ToBars(kind string, raw interface{}, width int, top int) string {
	if kind != "methodHotspots" && kind != "threadAnalysis" {
		return ""
	}
	topMap, ok := raw.(map[string]interface{})
	if !ok {
		return ""
	}
	result, _ := topMap["result"].(map[string]interface{})
	rows := hotspotRows(result)
	if len(rows) == 0 {
		return ""
	}

	n := 20
	if top > 0 && top < n {
		n = top
	}
	if len(rows) < n {
		n = len(rows)
	}
	rows = rows[:n]

	// Per-type max across visible rows (for independent normalization).
	maxPerType := make([]int, len(barTypes))
	for _, row := range rows {
		for i, t := range barTypes {
			if v := t.get(row); v > maxPerType[i] {
				maxPerType[i] = v
			}
		}
	}

	colorOn := output.ColorEnabled()
	var sb strings.Builder

	// Legend
	labels := []string{"Running", "Lock", "Net I/O", "Disk I/O", "Wait"}
	if colorOn {
		for i, t := range barTypes {
			if maxPerType[i] > 0 {
				fmt.Fprintf(&sb, "\033[38;5;%sm█ %s\033[0m  ", t.color, labels[i])
			}
		}
		sb.WriteString("\n\n")
	} else {
		for i, t := range barTypes {
			if maxPerType[i] > 0 {
				fmt.Fprintf(&sb, "%s=%s  ", t.name, labels[i])
			}
		}
		sb.WriteString("\n\n")
	}

	for _, row := range rows {
		name := truncate(row.Class+"."+row.Method, nameColWidth)
		sb.WriteString(name)
		sb.WriteString(strings.Repeat(" ", nameColWidth-len([]rune(name))+1))

		for i, t := range barTypes {
			if maxPerType[i] == 0 {
				continue
			}
			v := t.get(row)
			filled := int(float64(v) / float64(maxPerType[i]) * float64(miniBarWidth))
			bar := strings.Repeat("█", filled) + strings.Repeat("░", miniBarWidth-filled)
			if colorOn {
				fmt.Fprintf(&sb, "\033[38;5;%sm%s\033[0m %5d  ", t.color, bar, v)
			} else {
				fmt.Fprintf(&sb, "%s:%s %5d  ", t.name, bar, v)
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
