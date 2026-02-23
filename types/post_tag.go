package types

type CreatePostTags struct {
	PostID    string
	CommentID *string
	Tags      []string
}
