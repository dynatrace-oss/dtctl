package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

const treeMinPct = 0.5 // skip nodes below this % of total

// ToTree renders an indented call-tree for hotspot/thread kinds.
func ToTree(kind string, raw interface{}, width int) string {
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

	type tNode struct {
		id       string
		label    string
		running  int
		children []*tNode
	}

	nodeMap := make(map[string]*tNode, len(nodes))
	for _, n := range nodes {
		nm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		id := nodeIDStr(nm["id"])
		nodeMap[id] = &tNode{
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

	var sortChildren func(n *tNode)
	sortChildren = func(n *tNode) {
		sort.Slice(n.children, func(i, j int) bool {
			return n.children[i].running > n.children[j].running
		})
		for _, c := range n.children {
			sortChildren(c)
		}
	}
	sortChildren(root)

	colorOn := output.ColorEnabled()
	const nameWidth = 45
	const miniBar = 10

	var sb strings.Builder
	var dfs func(n *tNode, prefix string, isLast bool)
	dfs = func(n *tNode, prefix string, isLast bool) {
		// virtual root: skip to children
		if n.label == "" && n.id == rootID {
			for i, c := range n.children {
				dfs(c, "", i == len(n.children)-1)
			}
			return
		}

		pct := float64(n.running) / float64(total) * 100
		if pct < treeMinPct {
			return
		}

		connector := "├─ "
		if isLast {
			connector = "└─ "
		}

		label := truncate(n.label, nameWidth)
		labelPad := nameWidth - len([]rune(label))
		filled := int(pct / 100 * float64(miniBar))
		bar := strings.Repeat("█", filled) + strings.Repeat("░", miniBar-filled)

		if colorOn {
			heat := pct / 100
			fmt.Fprintf(&sb, "%s%s%s%s  %5d  %5.1f%%  \033[%sm%s\033[0m\n",
				prefix, connector, label, strings.Repeat(" ", labelPad),
				n.running, pct,
				heatColor(heat), bar,
			)
		} else {
			fmt.Fprintf(&sb, "%s%s%s%s  %5d  %5.1f%%  %s\n",
				prefix, connector, label, strings.Repeat(" ", labelPad),
				n.running, pct, bar,
			)
		}

		childPrefix := prefix
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}
		for i, c := range n.children {
			dfs(c, childPrefix, i == len(n.children)-1)
		}
	}

	dfs(root, "", true)
	return sb.String()
}
