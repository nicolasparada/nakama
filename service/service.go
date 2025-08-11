package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nicolasparada/nakama/cockroach"
	"github.com/nicolasparada/nakama/minio"
)

type Service struct {
	Cockroach         *cockroach.Cockroach
	Minio             *minio.Minio
	Errs              chan error
	BaseContext       context.Context
	BackgroundTimeout time.Duration

	wg sync.WaitGroup
}

func (svc *Service) background(fn func(ctx context.Context) error) {
	svc.wg.Add(1)
	go func() {
		defer svc.wg.Done()

		defer func() {
			if r := recover(); r != nil {
				svc.Errs <- fmt.Errorf("panic occurred: %v", r)
			}
		}()

		ctx, cancel := context.WithTimeout(svc.BaseContext, svc.BackgroundTimeout)
		defer cancel()

		if err := fn(ctx); err != nil {
			svc.Errs <- err
		}
	}()
}

func (svc *Service) Wait() {
	svc.wg.Wait()
}
