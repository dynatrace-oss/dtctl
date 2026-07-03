package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// ToStackTree renders the full call tree with resolved labels and per-node sample counts.
// Unlike -o tree it applies no threshold filtering and shows all sample types.
// appPrefix is the only package root shown when --app-only is set.
const appPrefix = "com.dynatrace."

func isSystemFrame(label string) bool {
	return !strings.HasPrefix(label, appPrefix)
}

// commonDotPrefix returns the longest shared dot-segment prefix across all labels.
func commonDotPrefix(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	segs := strings.Split(labels[0], ".")
	for _, l := range labels[1:] {
		ls := strings.Split(l, ".")
		i := 0
		for i < len(segs) && i < len(ls) && segs[i] == ls[i] {
			i++
		}
		segs = segs[:i]
	}
	// Keep only package segments (drop the last segment if it looks like a Class).
	// A segment is a package part if it's all lowercase.
	end := len(segs)
	for end > 0 && len(segs[end-1]) > 0 && segs[end-1][0] >= 'A' && segs[end-1][0] <= 'Z' {
		end--
	}
	segs = segs[:end]
	if len(segs) == 0 {
		return ""
	}
	return strings.Join(segs, ".") + "."
}

func ToStackTree(kind string, raw interface{}, width, maxDepth int, appOnly bool) string {
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

	type stNode struct {
		id       string
		label    string
		samples  map[string]int
		children []*stNode
	}

	nodeMap := make(map[string]*stNode, len(nodes))
	for _, n := range nodes {
		nm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		id := nodeIDStr(nm["id"])
		nodeMap[id] = &stNode{
			id:      id,
			label:   nodeLabel(nm),
			samples: nodeSamples(nm),
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

	total := root.samples["RUNNING"]
	if total == 0 {
		for _, c := range root.children {
			total += c.samples["RUNNING"]
		}
	}

	var sortChildren func(n *stNode)
	sortChildren = func(n *stNode) {
		sort.Slice(n.children, func(i, j int) bool {
			return n.children[i].samples["RUNNING"] > n.children[j].samples["RUNNING"]
		})
		for _, c := range n.children {
			sortChildren(c)
		}
	}
	sortChildren(root)

	colorOn := output.ColorEnabled()

	// sample type display order with colors matching bars.go
	type sampleDef struct{ key, short, color string }
	sampleDefs := []sampleDef{
		{"RUNNING", "R", "39"},
		{"LOCK", "L", "196"},
		{"NET_IO", "N", "226"},
		{"DISK_IO", "D", "46"},
		{"WAIT", "W", "244"},
	}

	formatSamples := func(s map[string]int) string {
		var parts []string
		for _, def := range sampleDefs {
			v := s[def.key]
			if v == 0 {
				continue
			}
			if colorOn {
				parts = append(parts, fmt.Sprintf("\033[38;5;%sm%s\033[0m:%d", def.color, def.short, v))
			} else {
				parts = append(parts, fmt.Sprintf("%s:%d", def.short, v))
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "  [" + strings.Join(parts, "  ") + "]"
	}

	// Collect visible labels to find the longest common package prefix to strip.
	var visibleLabels []string
	var collectLabels func(n *stNode)
	collectLabels = func(n *stNode) {
		if n.label != "" && n.id != rootID && !(appOnly && isSystemFrame(n.label)) {
			visibleLabels = append(visibleLabels, n.label)
		}
		for _, c := range n.children {
			collectLabels(c)
		}
	}
	collectLabels(root)
	stripPrefix := commonDotPrefix(visibleLabels)

	var sb strings.Builder
	if stripPrefix != "" {
		if colorOn {
			fmt.Fprintf(&sb, "\033[2m[%s]\033[0m\n", stripPrefix)
		} else {
			fmt.Fprintf(&sb, "[%s]\n", stripPrefix)
		}
	}

	// dfs(n, prefix, connector):
	//   prefix   — continuation lines already established by ancestors
	//   connector — "├─ ", "└─ ", or "↳  " (always 3 visible chars wide)
	//
	// Single-child chains use "↳  " and do NOT grow the prefix, so indentation
	// only increases at real forks.
	var dfs func(n *stNode, prefix, connector string, depth int)
	dfs = func(n *stNode, prefix, connector string, depth int) {
		if maxDepth > 0 && depth > maxDepth {
			return
		}
		// skip virtual root
		if n.label == "" && n.id == rootID {
			for i, c := range n.children {
				conn := "├─ "
				if i == len(n.children)-1 {
					conn = "└─ "
				}
				dfs(c, "", conn, depth+1)
			}
			return
		}

		if appOnly && isSystemFrame(n.label) {
			// transparent: pass children through without rendering this node
			switch len(n.children) {
			case 1:
				dfs(n.children[0], prefix, connector, depth)
			default:
				var childBase string
				if connector == "├─ " {
					childBase = prefix + "│  "
				} else {
					childBase = prefix + "   "
				}
				for i, c := range n.children {
					conn := "├─ "
					if i == len(n.children)-1 {
						conn = "└─ "
					}
					dfs(c, childBase, conn, depth)
				}
			}
			return
		}

		label := strings.TrimPrefix(n.label, stripPrefix)
		samples := formatSamples(n.samples)

		fmt.Fprintf(&sb, "%s%s%s%s\n", prefix, connector, label, samples)

		switch len(n.children) {
		case 0:
			// leaf
		case 1:
			// single child: no new indent, use ↳ — but still advance the
			// prefix for the parent's branch line if this was a non-last fork arm.
			var childPrefix string
			switch connector {
			case "├─ ":
				childPrefix = prefix + "│  "
			case "└─ ":
				childPrefix = prefix + "   "
			default: // "↳  " — already collapsed, prefix stays put
				childPrefix = prefix
			}
			dfs(n.children[0], childPrefix, "↳  ", depth+1)
		default:
			// real fork: indent under this node
			var childBase string
			switch connector {
			case "├─ ":
				childBase = prefix + "│  "
			default: // "└─ " or "↳  "
				childBase = prefix + "   "
			}
			for i, c := range n.children {
				conn := "├─ "
				if i == len(n.children)-1 {
					conn = "└─ "
				}
				dfs(c, childBase, conn, depth+1)
			}
		}
	}

	dfs(root, "", "", 0)
	return sb.String()
}
