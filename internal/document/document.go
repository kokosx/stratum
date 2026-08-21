package document

import "encoding/json"

type Document struct {
	Version int    `json:"version"`
	Nodes   []Node `json:"nodes"`
}

type Node struct {
	ID       string          `json:"id"`
	Block    string          `json:"block"`
	Version  int             `json:"version"`
	Props    json.RawMessage `json:"props"`
	Settings json.RawMessage `json:"settings,omitempty"`
	Children []Node          `json:"children,omitempty"`
}
