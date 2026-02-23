package service

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nakamauwu/nakama/cockroach"
	"github.com/ory/dockertest/v3"
)

var (
	testDB        *pgxpool.Pool
	testCockroach *cockroach.Cockroach
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	var skipIntegration bool
	flag.BoolVar(&skipIntegration, "skip-integration", false, "Skip integration tests docker setup")
	flag.Parse()

	if skipIntegration || testing.Short() {
		return m.Run()
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		fmt.Printf("could not create docker pool: %v\n", err)
		return 1
	}

	var cleanup func() error
	testDB, cleanup, err = setupTestDB(pool)
	if err != nil {
		fmt.Printf("could not setup test db: %v\n", err)
		return 1
	}
	testCockroach = cockroach.New(testDB)

	_, err = testDB.Exec(context.Background(), cockroach.Schema)
	if err != nil {
		fmt.Printf("could not exec schema: %v\n", err)
		return 1
	}

	defer func() {
		if err := cleanup(); err != nil {
			fmt.Printf("could not cleanup cockroach container: %v\n", err)
		}
	}()

	return m.Run()
}

func setupTestDB(pool *dockertest.Pool) (*pgxpool.Pool, func() error, error) {
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "cockroachdb/cockroach",
		Tag:        "latest",
		Cmd:        []string{"start-single-node", "--insecure"},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("could not create cockroach resource: %w", err)
	}

	var db *pgxpool.Pool
	err = pool.Retry(func() (err error) {
		hostPort := resource.GetHostPort("26257/tcp")
		db, err = pgxpool.New(context.Background(), "postgresql://root@"+hostPort+"/nakama?sslmode=disable")
		if err != nil {
			return fmt.Errorf("could not open db: %w", err)
		}

		// do not close db

		if err = db.Ping(context.Background()); err != nil {
			return fmt.Errorf("could not ping db: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return db, func() error {
		return pool.Purge(resource)
	}, nil
}
