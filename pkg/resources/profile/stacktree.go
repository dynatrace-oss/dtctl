package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// appPrefix is the only package root shown when --app-only is set.
const appPrefix = "com.dynatrace."

func isSystemFrame(label string) bool {
	return !strings.HasPrefix(label, appPrefix)
}

// abbreviateLabel shortens each lowercase package segment to its first character,
// leaving class names (uppercase-initial) and the method suffix intact.
// e.g. com.dynatrace.easytravel.business.webservice.JourneyService.findJourneys()
//   →  c.d.e.b.w.JourneyService.findJourneys()
func abbreviateLabel(label string) string {
	segs := strings.Split(label, ".")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		if len(s) > 0 && s[0] >= 'a' && s[0] <= 'z' {
			out = append(out, string(s[0]))
		} else {
			out = append(out, s)
		}
	}
	return strings.Join(out, ".")
}

// ToStackTree renders the full call tree with resolved labels and per-node sample counts.
// Unlike -o tree it applies no threshold filtering and shows all sample types.
func ToStackTree(kind string, raw interface{}, width, maxDepth int, appOnly, abbrev bool) string {
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
			if v := s[def.key]; v != 0 {
				if colorOn {
					parts = append(parts, fmt.Sprintf("\033[38;5;%sm%s\033[0m:%d", def.color, def.short, v))
				} else {
					parts = append(parts, fmt.Sprintf("%s:%d", def.short, v))
				}
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "  [" + strings.Join(parts, "  ") + "]"
	}

	var sb strings.Builder

	var dfs func(n *stNode, prefix, connector string, depth int)
	dfs = func(n *stNode, prefix, connector string, depth int) {
		if maxDepth > 0 && depth > maxDepth {
			return
		}

		// Virtual root: recurse directly into children.
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

		// System frame passthrough (appOnly mode).
		if appOnly && isSystemFrame(n.label) {
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

		label := n.label
		if abbrev {
			label = abbreviateLabel(label)
		}
		fmt.Fprintf(&sb, "%s%s%s%s\n", prefix, connector, label, formatSamples(n.samples))

		switch len(n.children) {
		case 0:
		case 1:
			var childPrefix string
			switch connector {
			case "├─ ":
				childPrefix = prefix + "│  "
			case "└─ ":
				childPrefix = prefix + "   "
			default: // "↳  "
				childPrefix = prefix
			}
			dfs(n.children[0], childPrefix, "↳  ", depth+1)
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
				dfs(c, childBase, conn, depth+1)
			}
		}
	}

	dfs(root, "", "", 0)
	return sb.String()
}
