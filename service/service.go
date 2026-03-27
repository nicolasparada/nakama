package service

import (
	_ "embed"
	"html/template"
	"net/url"
	"sync"

	"github.com/go-kit/log"

	"github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/mailing"
	"github.com/nakamauwu/nakama/minio"
	"github.com/nakamauwu/nakama/pubsub"
)

// Service contains the core business logic separated from the transport layer.
// You can use it to back a REST, gRPC or GraphQL API.
// You must call RunBackgroundJobs afterward.
type Service struct {
	Logger           log.Logger
	Cockroach        *cockroach.Cockroach
	Sender           mailing.Sender
	Origin           *url.URL
	TokenKey         string
	PubSub           pubsub.PubSub
	MinioStore       *minio.Store
	ObjectsBaseURL   string
	DisabledDevLogin bool
	AllowedOrigins   []string
	VAPIDPrivateKey  string
	VAPIDPublicKey   string

	magicLinkTmplOncer sync.Once
	magicLinkTmpl      *template.Template
}
