package types

import "io"

type Media struct {
	Path        string `json:"path"`
	Width       uint   `json:"width"`
	Height      uint   `json:"height"`
	ContentType string `json:"contentType"`

	reader io.ReadSeeker `json:"-"`
}

func (m *Media) SetReader(r io.ReadSeeker) {
	m.reader = r
}

func (m Media) Reader() io.ReadSeeker {
	return m.reader
}
