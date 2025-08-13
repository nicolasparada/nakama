package preview

import (
	"context"
	"fmt"
	"sync"

	"github.com/nicolasparada/nakama/opengraph"
)

// Result is the outcome for a single URL.
type Result struct {
	URL  string
	Data opengraph.OpenGraph
	Err  error
}

type Results []Result

func (r Results) IsEmpty() bool {
	for _, res := range r {
		if !res.Data.IsEmpty() {
			return false
		}
	}
	return true
}

type Monitored struct {
	fetcher *Fetcher
	errs    chan error
}

func NewMonitored(fetcher *Fetcher) *Monitored {
	return &Monitored{
		fetcher: fetcher,
		errs:    make(chan error, 1),
	}
}

func (m *Monitored) Errs() <-chan error {
	return m.errs
}

func (m *Monitored) Close() {
	close(m.errs)
}

func (m *Monitored) Fetch(ctx context.Context, urls []string) Results {
	if len(urls) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	results := make(Results, len(urls))

	for i, url := range urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()

			defer func() {
				if rcv := recover(); rcv != nil {
					select {
					case m.errs <- fmt.Errorf("panic occurred while fetching preview %q: %v", url, rcv):
					default:
					}
				}
			}()

			data, err := m.fetcher.Get(ctx, url)
			if err != nil {
				select {
				case m.errs <- fmt.Errorf("failed to fetch preview %s: %w", url, err):
				default:
				}
			}

			results[i] = Result{URL: url, Data: data, Err: err}
		}(i, url)
	}

	wg.Wait()

	return results
}
