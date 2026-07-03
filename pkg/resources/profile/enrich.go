package profile

import (
	"fmt"
	"sort"
	"strings"
)

// HotspotRow is the table representation of a single method hotspot.
type HotspotRow struct {
	Class   string `json:"class"   table:"CLASS"`
	Method  string `json:"method"  table:"METHOD"`
	Running int    `json:"running" table:"RUNNING"`
	Lock    int    `json:"lock"    table:"LOCK"`
	NetIO   int    `json:"netIO"   table:"NET_IO"`
	DiskIO  int    `json:"diskIO"  table:"DISK_IO"`
	Wait    int    `json:"wait"    table:"WAIT"`
}

// MemoryTypeRow is the table representation of a single allocated type.
type MemoryTypeRow struct {
	Type          string `json:"type"          table:"TYPE"`
	AllocCount    int    `json:"allocCount"    table:"ALLOC COUNT"`
	AllocSize     int    `json:"allocSize"     table:"ALLOC SIZE"`
	SurvivorCount int    `json:"survivorCount" table:"SURVIVOR COUNT"`
	SurvivorSize  int    `json:"survivorSize"  table:"SURVIVOR SIZE"`
}

// EnrichResult injects analytical summaries (hotPaths for CPU kinds,
// topTypes for memory kinds) that agents and users can consume directly.
func EnrichResult(kind string, raw interface{}) interface{} {
	top, ok := raw.(map[string]interface{})
	if !ok {
		return raw
	}
	result, ok := top["result"].(map[string]interface{})
	if !ok {
		return raw
	}

	switch kind {
	case "methodHotspots", "threadAnalysis":
		if paths := buildHotPaths(result); len(paths) > 0 {
			top["hotPaths"] = paths
		}
	}
	return top
}

// LimitRows truncates rows to n if n > 0.
func LimitRows(rows interface{}, n int) interface{} {
	if n <= 0 {
		return rows
	}
	switch v := rows.(type) {
	case []HotspotRow:
		if n < len(v) {
			return v[:n]
		}
	case []MemoryTypeRow:
		if n < len(v) {
			return v[:n]
		}
	}
	return rows
}

// ToTableRows converts a profile result to a slice of table-renderable rows.
// Returns nil if the kind/data isn't suited for table display.
func ToTableRows(kind string, raw interface{}) interface{} {
	top, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	result, _ := top["result"].(map[string]interface{})

	switch kind {
	case "methodHotspots", "threadAnalysis":
		return hotspotRows(result)
	case "memoryAllocation", "memoryAllocationDetails":
		return memoryTypeRows(result)
	}
	return nil
}

// --- hotspots / thread analysis ---

func hotspotRows(result map[string]interface{}) []HotspotRow {
	nodes := dataNodes(result)
	if len(nodes) == 0 {
		return nil
	}
	// skip the virtual root (empty label)
	var rows []HotspotRow
	for _, raw := range nodes {
		n, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		label := nodeLabel(n)
		if label == "" {
			continue
		}
		s := nodeSamples(n)
		rows = append(rows, HotspotRow{
			Class:   nodeClass(n),
			Method:  nodeMethod(n),
			Running: s["RUNNING"],
			Lock:    s["LOCK"],
			NetIO:   s["NET_IO"],
			DiskIO:  s["DISK_IO"],
			Wait:    s["WAIT"],
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Running > rows[j].Running })
	return rows
}

// --- memory ---

func memoryTypeRows(result map[string]interface{}) []MemoryTypeRow {
	ar, _ := result["analysisResult"].(map[string]interface{})
	if ar == nil {
		return nil
	}
	typesRaw, _ := ar["types"].([]interface{})
	if len(typesRaw) == 0 {
		return nil
	}
	rows := make([]MemoryTypeRow, 0, len(typesRaw))
	for _, t := range typesRaw {
		tm, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, MemoryTypeRow{
			Type:          strVal(tm, "typeName"),
			AllocCount:    intVal(tm, "allocationCount"),
			AllocSize:     intVal(tm, "allocationSize"),
			SurvivorCount: intVal(tm, "survivorCount"),
			SurvivorSize:  intVal(tm, "survivorSize"),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].AllocSize > rows[j].AllocSize })
	return rows
}

// --- hotPaths builder ---

type nodeInfo struct {
	id       string
	label    string
	childIDs []string
	running  int
}

func buildHotPaths(result map[string]interface{}) []map[string]interface{} {
	nodes := dataNodes(result)
	if len(nodes) == 0 {
		return nil
	}

	nodeMap := make(map[string]*nodeInfo, len(nodes))
	parentOf := make(map[string]string, len(nodes))
	rootID := nodeIDStr(result["dataRootNodeId"])

	for _, n := range nodes {
		nm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		id := nodeIDStr(nm["id"])
		info := &nodeInfo{
			id:      id,
			label:   nodeLabel(nm),
			running: nodeSamples(nm)["RUNNING"],
		}
		if cids, ok := nm["childIds"].([]interface{}); ok {
			for _, c := range cids {
				if s, ok := c.(string); ok {
					info.childIDs = append(info.childIDs, s)
					parentOf[s] = id
				}
			}
		}
		nodeMap[id] = info
	}

	// Total = max running count across all nodes (root often has 0; top real node has the max).
	totalSamples := 0
	for _, n := range nodeMap {
		if n.running > totalSamples {
			totalSamples = n.running
		}
	}

	// Find leaves sorted by running samples.
	var leaves []*nodeInfo
	for _, n := range nodeMap {
		if len(n.childIDs) == 0 && n.label != "" {
			leaves = append(leaves, n)
		}
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].running > leaves[j].running })
	if len(leaves) > 5 {
		leaves = leaves[:5]
	}

	paths := make([]map[string]interface{}, 0, len(leaves))
	for _, leaf := range leaves {
		chain := traceToRoot(leaf.id, rootID, nodeMap, parentOf)
		pct := 0.0
		if totalSamples > 0 {
			pct = float64(leaf.running) / float64(totalSamples) * 100
		}
		paths = append(paths, map[string]interface{}{
			"path":           strings.Join(chain, " → "),
			"runningSamples": leaf.running,
			"pct":            fmt.Sprintf("%.1f%%", pct),
		})
	}
	return paths
}

func traceToRoot(leafID, rootID string, nodes map[string]*nodeInfo, parentOf map[string]string) []string {
	var chain []string
	cur := leafID
	for cur != "" && cur != rootID {
		n := nodes[cur]
		if n == nil {
			break
		}
		if n.label != "" {
			chain = append(chain, n.label)
		}
		cur = parentOf[cur]
	}
	// reverse: root → leaf
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// --- helpers ---

func dataNodes(result map[string]interface{}) []interface{} {
	if result == nil {
		return nil
	}
	nodes, _ := result["dataNodes"].([]interface{})
	return nodes
}

func nodeLabel(n map[string]interface{}) string {
	cp, _ := n["classPath"].(string)
	cn, _ := n["className"].(string)
	mn, _ := n["methodName"].(string)
	if cn == "" || mn == "" {
		return ""
	}
	if cp != "" {
		return cp + "." + cn + "." + mn + "()"
	}
	return cn + "." + mn + "()"
}

func nodeClass(n map[string]interface{}) string {
	cp, _ := n["classPath"].(string)
	cn, _ := n["className"].(string)
	if cp != "" {
		return cp + "." + cn
	}
	return cn
}

func nodeMethod(n map[string]interface{}) string {
	mn, _ := n["methodName"].(string)
	return mn + "()"
}

func nodeSamples(n map[string]interface{}) map[string]int {
	out := map[string]int{"RUNNING": 0, "LOCK": 0, "NET_IO": 0, "DISK_IO": 0, "WAIT": 0}
	s, ok := n["samples"].(map[string]interface{})
	if !ok {
		return out
	}
	for k := range out {
		if v, ok := s[k].(float64); ok {
			out[k] = int(v)
		}
	}
	return out
}

func nodeIDStr(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	}
	return ""
}

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func intVal(m map[string]interface{}, key string) int {
	v, _ := m[key].(float64)
	return int(v)
}
