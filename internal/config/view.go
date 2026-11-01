// Package config holds the user's display preferences.
package config

// View says what the status line shows.
type View struct {
	Density string
	Hidden  []string
}
