package types

import "io"

type Attachment struct {
	reader      io.ReadSeeker
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	FileSize    uint64 `json:"fileSize"`
	Width       uint32 `json:"width"`
	Height      uint32 `json:"height"`
}

func (a *Attachment) SetReader(reader io.ReadSeeker) {
	a.reader = reader
}

func (a *Attachment) Reader() io.ReadSeeker {
	return a.reader
}
