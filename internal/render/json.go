package render

import "encoding/json"

// JSON prints lines for hosts that draw the segments themselves.
func JSON(lines []Line) []byte {
	if lines == nil {
		return []byte("[]")
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return []byte("[]")
	}
	return b
}
