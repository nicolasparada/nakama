package types

import "github.com/nicolasparada/go-errs"

type ToggleSubscription struct {
	PostID string `json:"-"`
	userID string
}

func (in *ToggleSubscription) SetUserID(userID string) {
	in.userID = userID
}

func (in ToggleSubscription) UserID() string {
	return in.userID
}

func (in *ToggleSubscription) Validate() error {
	if !ValidUUIDv4(in.PostID) {
		return errs.InvalidArgumentError("invalid post ID")
	}

	return nil
}

type ToggledSubscription struct {
	Subscribed bool `json:"subscribed"`
}
