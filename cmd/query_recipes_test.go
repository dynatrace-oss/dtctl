package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	builtinrecipes "github.com/dynatrace-oss/dtctl/recipes"
)

// TestQueryHintsDoNotCountOneWordTwice: "event" and "events" in a generic
// events query are one concept, so they do not make k8s-warning-events a
// match "for this question"; a Kubernetes events query still gets it.
func TestQueryHintsDoNotCountOneWordTwice(t *testing.T) {
	book := recipes.Loader{Builtin: recipes.FileLayer{Layer: recipes.LayerBuiltin, FS: builtinrecipes.FS(), Root: "builtin"}}.Load()
	named := func(query string) string {
		return strings.Join(queryRecipeHints(book, query, "", recipeHintOK), "\n")
	}
	assert.NotContains(t, named(`fetch events, from: now()-24h | summarize n = count(), by:{event.type}`), "k8s-warning-events")
	assert.Contains(t, named(`fetch events | filter event.kind == "K8S_EVENT" | summarize n = count(), by:{k8s.namespace.name, k8s.event.reason}`), "k8s-warning-events")
}
