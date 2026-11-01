// Package config holds the user's display preferences.
package config

// View says what the status line shows.
type View struct {
	Density  string            `json:"density,omitempty"` // full, compact or minimal
	Order    []string          `json:"order,omitempty"`   // badge ids, listed ones first
	Hidden   []string          `json:"hidden,omitempty"`  // item ids to leave out
	Variants map[string]string `json:"variants,omitempty"`
}

// Item ids for Order and Hidden: the integrations' badges, then the session's.
const (
	Ponytail  = "ponytail"
	Caveman   = "caveman"
	RTK       = "rtk"
	ClaudeMem = "claude-mem"
	Model     = "model"
	Context   = "context"
	Output    = "output"
	Cost      = "cost"
	Cache     = "cache"
)

// Items lists every id Order and Hidden understand.
func Items() []string {
	return []string{Ponytail, Caveman, RTK, ClaudeMem, Model, Context, Output, Cost, Cache}
}

// Shows reports whether the item is not hidden.
func (v View) Shows(id string) bool {
	for _, h := range v.Hidden {
		if h == id {
			return false
		}
	}
	return true
}
