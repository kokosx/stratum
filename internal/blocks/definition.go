package blocks

type BlockKey struct {
	Name    string
	Version int64
}

type Definition struct {
	Namespace   string
	Name        string
	Version     int64
	DisplayName string
	Description string
	Schema      Schema
	Template    string
	Styles      string
	Source      string
	Enabled     bool
}

type EditorDefinition struct {
	Block       string `json:"block"`
	Version     int64  `json:"version"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	Schema      Schema `json:"schema"`
}
