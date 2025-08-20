package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nicolasparada/nakama/cockroach"
	"github.com/nicolasparada/nakama/minio"
	"github.com/nicolasparada/nakama/preview"
)

type Config struct {
	Cockroach         *cockroach.Cockroach
	Minio             *minio.Minio
	Preview           *preview.Monitored
	BaseCtx           context.Context
	BackgroundTimeout time.Duration
}

type Service struct {
	Cockroach *cockroach.Cockroach
	Minio     *minio.Minio
	Preview   *preview.Monitored

	baseCtx           context.Context
	backgroundTimeout time.Duration
	wg                sync.WaitGroup
	errs              chan error
}

func New(cfg *Config) *Service {
	return &Service{
		Cockroach: cfg.Cockroach,
		Minio:     cfg.Minio,
		Preview:   cfg.Preview,

		baseCtx:           cfg.BaseCtx,
		backgroundTimeout: cfg.BackgroundTimeout,
		errs:              make(chan error, 1),
	}
}

func (svc *Service) Errs() <-chan error {
	return svc.errs
}

func (svc *Service) Close() error {
	svc.wg.Wait()
	close(svc.errs)
	return nil
}

func (svc *Service) background(fn func(ctx context.Context) error) {
	svc.wg.Go(func() {
		defer func() {
			if rcv := recover(); rcv != nil {
				select {
				case svc.errs <- fmt.Errorf("service background panic: %v", rcv):
				default:
				}
			}
		}()

		ctx, cancel := context.WithTimeout(svc.baseCtx, svc.backgroundTimeout)
		defer cancel()

		if err := fn(ctx); err != nil {
			select {
			case svc.errs <- fmt.Errorf("service background error: %w", err):
			default:
			}
		}
	})
}
