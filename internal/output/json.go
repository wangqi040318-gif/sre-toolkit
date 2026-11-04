package output

import (
	"encoding/json"
	"io"
)

// WriteJSON prints doc as indented JSON with HTML escaping disabled so
// kubectl snippets keep their angle brackets.
func WriteJSON(w io.Writer, doc Document) error {
	doc = doc.normalized()
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
