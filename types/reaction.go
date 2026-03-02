package types

import (
	"github.com/nakamauwu/nakama/emoji"
	"github.com/nicolasparada/go-errs"
)

type ReactionKind string

const (
	ReactionKindEmoji ReactionKind = "emoji"
)

func (k ReactionKind) IsValid() bool {
	switch k {
	case ReactionKindEmoji:
		return true
	default:
		return false
	}
}

func (k ReactionKind) Validate() error {
	if !k.IsValid() {
		return errs.InvalidArgumentError("invalid reaction kind")
	}

	return nil
}

func (k ReactionKind) String() string {
	return string(k)
}

type Reaction struct {
	Kind     ReactionKind `json:"kind"`
	Reaction string       `json:"reaction"`
	Count    uint64       `json:"count"`
	Reacted  *bool        `json:"reacted,omitempty"`
}

type ReactionInput struct {
	Kind     ReactionKind `json:"kind"`
	Reaction string       `json:"reaction"`
}

type TogglePostReaction struct {
	PostID   string       `json:"-"`
	Kind     ReactionKind `json:"kind"`
	Reaction string       `json:"reaction"`
	userID   string
}

func (in *TogglePostReaction) SetUserID(userID string) {
	in.userID = userID
}

func (in TogglePostReaction) UserID() string {
	return in.userID
}

func (in *TogglePostReaction) Validate() error {
	if !ValidUUIDv4(in.PostID) {
		return errs.InvalidArgumentError("invalid post ID")
	}

	if err := in.Kind.Validate(); err != nil {
		return err
	}

	switch in.Kind {
	case ReactionKindEmoji:
		if !emoji.IsValid(in.Reaction) {
			return errs.InvalidArgumentError("invalid reaction")
		}
	}

	return nil
}

type ToggleCommentReaction struct {
	CommentID string       `json:"-"`
	Kind      ReactionKind `json:"kind"`
	Reaction  string       `json:"reaction"`
	userID    string
}

func (in *ToggleCommentReaction) SetUserID(userID string) {
	in.userID = userID
}

func (in ToggleCommentReaction) UserID() string {
	return in.userID
}

func (in *ToggleCommentReaction) Validate() error {
	if !ValidUUIDv4(in.CommentID) {
		return errs.InvalidArgumentError("invalid comment ID")
	}

	if err := in.Kind.Validate(); err != nil {
		return err
	}

	switch in.Kind {
	case ReactionKindEmoji:
		if !emoji.IsValid(in.Reaction) {
			return errs.InvalidArgumentError("invalid reaction")
		}
	}

	return nil
}
