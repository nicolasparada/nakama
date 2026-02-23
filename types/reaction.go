package types

type Reaction struct {
	Type     string `json:"type"`
	Reaction string `json:"reaction"`
	Count    uint64 `json:"count"`
	Reacted  *bool  `json:"reacted,omitempty"`
}

type ReactionInput struct {
	Type     string `json:"type"`
	Reaction string `json:"reaction"`
}
