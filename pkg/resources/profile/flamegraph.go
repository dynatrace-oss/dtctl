package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

const minBlockWidth = 3 // blocks narrower than this are skipped

// ToFlamegraph renders a Unicode flamegraph for hotspot/thread kinds.
// Returns an empty string for unsupported kinds or missing data.
func ToFlamegraph(kind string, raw interface{}, width int) string {
	if kind != "methodHotspots" && kind != "threadAnalysis" {
		return ""
	}
	top, ok := raw.(map[string]interface{})
	if !ok {
		return ""
	}
	result, _ := top["result"].(map[string]interface{})
	nodes := dataNodes(result)
	if len(nodes) == 0 {
		return ""
	}

	rootID := nodeIDStr(result["dataRootNodeId"])

	type fgNode struct {
		id       string
		label    string
		running  int
		children []*fgNode
		startX   int
		endX     int
		depth    int
	}

	nodeMap := make(map[string]*fgNode, len(nodes))
	for _, n := range nodes {
		nm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		id := nodeIDStr(nm["id"])
		nodeMap[id] = &fgNode{
			id:      id,
			label:   fgLabel(nm),
			running: nodeSamples(nm)["RUNNING"],
		}
	}
	for _, n := range nodes {
		nm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		parent := nodeMap[nodeIDStr(nm["id"])]
		if cids, ok := nm["childIds"].([]interface{}); ok {
			for _, c := range cids {
				if s, ok := c.(string); ok {
					if child := nodeMap[s]; child != nil {
						parent.children = append(parent.children, child)
					}
				}
			}
		}
	}

	root := nodeMap[rootID]
	if root == nil {
		return ""
	}

	total := root.running
	if total == 0 {
		for _, c := range root.children {
			total += c.running
		}
	}
	if total == 0 {
		return ""
	}

	// Assign x positions recursively; children share parent's range proportionally.
	var assignX func(n *fgNode, depth int)
	assignX = func(n *fgNode, depth int) {
		n.depth = depth
		sort.Slice(n.children, func(i, j int) bool {
			return n.children[i].running > n.children[j].running
		})
		x := n.startX
		for _, child := range n.children {
			w := int(float64(child.running) / float64(total) * float64(width))
			if w < 1 && child.running > 0 {
				w = 1
			}
			child.startX = x
			child.endX = x + w
			x += w
			assignX(child, depth+1)
		}
	}
	root.startX = 0
	root.endX = width
	assignX(root, 0)

	// Collect nodes by depth level; skip virtual root (empty label) and too-narrow blocks.
	maxDepth := 0
	for _, n := range nodeMap {
		if n.depth > maxDepth {
			maxDepth = n.depth
		}
	}
	levels := make([][]*fgNode, maxDepth+1)
	for _, n := range nodeMap {
		if n.label == "" {
			continue
		}
		if n.endX-n.startX < minBlockWidth {
			continue
		}
		levels[n.depth] = append(levels[n.depth], n)
	}

	// Render bottom-up (root level at bottom, leaves at top).
	var sb strings.Builder
	colorOn := output.ColorEnabled()
	for d := maxDepth; d >= 0; d-- {
		level := levels[d]
		if len(level) == 0 {
			continue
		}
		sort.Slice(level, func(i, j int) bool { return level[i].startX < level[j].startX })

		cursor := 0
		for _, n := range level {
			w := n.endX - n.startX
			if n.startX > cursor {
				sb.WriteString(strings.Repeat(" ", n.startX-cursor))
			}
			label := centerLabel(n.label, w)
			heat := float64(n.running) / float64(total)
			if colorOn {
				fmt.Fprintf(&sb, "\033[%sm%s\033[0m", heatColor(heat), label)
			} else {
				sb.WriteString(label)
			}
			cursor = n.endX
		}
		if cursor < width {
			sb.WriteString(strings.Repeat(" ", width-cursor))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// centerLabel produces a block of exactly `width` runes: label centered, padded with █.
func centerLabel(label string, width int) string {
	runes := []rune(label)
	if len(runes) > width {
		if width <= 1 {
			return strings.Repeat("█", width)
		}
		runes = append(runes[:width-1], '…')
	}
	pad := width - len(runes)
	left := pad / 2
	right := pad - left
	return strings.Repeat("█", left) + string(runes) + strings.Repeat("█", right)
}

// heatColor returns an ANSI 256-color fg code for a 0..1 heat value (blue → red).
func heatColor(heat float64) string {
	stops := []int{21, 33, 39, 46, 82, 226, 214, 208, 202, 196}
	idx := int(heat * float64(len(stops)-1))
	if idx >= len(stops) {
		idx = len(stops) - 1
	}
	return fmt.Sprintf("38;5;%d", stops[idx])
}

func fgLabel(n map[string]interface{}) string {
	cn, _ := n["className"].(string)
	mn, _ := n["methodName"].(string)
	if cn == "" || mn == "" {
		return ""
	}
	return cn + "." + mn + "()"
}
