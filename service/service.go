package service

import (
	"github.com/nicolasparada/nakama/cockroach"
	"github.com/nicolasparada/nakama/minio"
)

type Service struct {
	Cockroach *cockroach.Cockroach
	Minio     *minio.Minio
}
