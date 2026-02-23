package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-kit/log"
	"github.com/gorilla/securecookie"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	cockroachpkg "github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/mailing"
	natspubsub "github.com/nakamauwu/nakama/pubsub/nats"
	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/storage"
	fsstorage "github.com/nakamauwu/nakama/storage/fs"
	s3storage "github.com/nakamauwu/nakama/storage/s3"
	httptransport "github.com/nakamauwu/nakama/transport/http"
)

func main() {
	_ = godotenv.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	if err := run(ctx, logger, os.Args[1:]); err != nil {
		_ = logger.Log("error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger log.Logger, args []string) error {
	var (
		port, _             = strconv.Atoi(env("PORT", "3000"))
		originStr           = env("ORIGIN", fmt.Sprintf("http://localhost:%d", port))
		dbURL               = env("DATABASE_URL", "postgresql://root@127.0.0.1:26257/nakama?sslmode=disable")
		execSchema, _       = strconv.ParseBool(env("EXEC_SCHEMA", "false"))
		tokenKey            = env("TOKEN_KEY", "supersecretkeyyoushouldnotcommit")
		natsURL             = env("NATS_URL", nats.DefaultURL)
		resendAPIKey        = os.Getenv("RESEND_API_KEY")
		smtpHost            = env("SMTP_HOST", "smtp.mailtrap.io")
		smtpPort, _         = strconv.Atoi(env("SMTP_PORT", "25"))
		smtpUsername        = os.Getenv("SMTP_USERNAME")
		smtpPassword        = os.Getenv("SMTP_PASSWORD")
		embedStaticFiles, _ = strconv.ParseBool(env("EMBED_STATIC", "false"))
		s3Secure, _         = strconv.ParseBool(env("S3_SECURE", "true"))
		s3Endpoint          = os.Getenv("S3_ENDPOINT")
		s3Region            = os.Getenv("S3_REGION")
		s3AccessKey         = os.Getenv("S3_ACCESS_KEY")
		s3SecretKey         = os.Getenv("S3_SECRET_KEY")
		avatarURLPrefix     = env("AVATAR_URL_PREFIX", originStr+"/img/avatars/")
		coverURLPrefix      = env("COVER_URL_PREFIX", originStr+"/img/covers/")
		mediaURLPrefix      = env("MEDIA_URL_PREFIX", originStr+"/img/media/")
		cookieHashKey       = env("COOKIE_HASH_KEY", "supersecretkeyyoushouldnotcommit")
		cookieBlockKey      = env("COOKIE_BLOCK_KEY", "supersecretkeyyoushouldnotcommit")
		githubClientID      = os.Getenv("GITHUB_CLIENT_ID")
		githubClientSecret  = os.Getenv("GITHUB_CLIENT_SECRET")
		googleClientID      = os.Getenv("GOOGLE_CLIENT_ID")
		googleClientSecret  = os.Getenv("GOOGLE_CLIENT_SECRET")
		disabledDevLogin, _ = strconv.ParseBool(os.Getenv("DISABLE_DEV_LOGIN"))
		allowedOrigins      = os.Getenv("ALLOWED_ORIGINS")
		vapidPrivateKey     = os.Getenv("VAPID_PRIVATE_KEY")
		vapidPublicKey      = os.Getenv("VAPID_PUBLIC_KEY")
	)

	fs := flag.NewFlagSet("nakama", flag.ExitOnError)
	fs.Usage = func() {
		fs.PrintDefaults()
		fmt.Println("\nDon't forget to set TOKEN_KEY, and RESEND_API_KEY or SMTP_USERNAME and SMTP_PASSWORD for real usage.")
	}
	fs.IntVar(&port, "port", port, "Port in which this server will run")
	fs.StringVar(&originStr, "origin", originStr, "URL origin for this service")
	fs.StringVar(&dbURL, "db", dbURL, "Database URL")
	fs.BoolVar(&execSchema, "exec-schema", execSchema, "Execute database schema")
	fs.StringVar(&natsURL, "nats", natsURL, "NATS URL")
	fs.StringVar(&smtpHost, "smtp-host", smtpHost, "SMTP server host")
	fs.IntVar(&smtpPort, "smtp-port", smtpPort, "SMTP server port")
	fs.BoolVar(&embedStaticFiles, "embed-static", embedStaticFiles, "Embed static files")
	fs.StringVar(&avatarURLPrefix, "avatar-url-prefix", avatarURLPrefix, "Avatar URL prefix")
	fs.StringVar(&coverURLPrefix, "cover-url-prefix", coverURLPrefix, "Cover URL prefix")
	fs.StringVar(&mediaURLPrefix, "media-url-prefix", mediaURLPrefix, "Media URL prefix")
	fs.StringVar(&cookieHashKey, "cookie-hash-key", cookieHashKey, "Cookie hash key. 32 or 64 bytes")
	fs.StringVar(&cookieBlockKey, "cookie-block-key", cookieBlockKey, "Cookie block key. 16, 24, or 32 bytes")
	fs.StringVar(&githubClientID, "github-client-id", githubClientID, "GitHub client ID")
	fs.StringVar(&googleClientID, "google-client-id", googleClientID, "Google client ID")
	fs.BoolVar(&disabledDevLogin, "disable-dev-login", disabledDevLogin, "Disable development login endpoint")
	fs.StringVar(&allowedOrigins, "allowed-origins", allowedOrigins, "Comma separated list of allowed origins")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("could not parse flags: %w", err)
	}

	origin, err := url.Parse(originStr)
	if err != nil || !origin.IsAbs() {
		return errors.New("invalid url origin")
	}

	if h := origin.Hostname(); h == "localhost" || h == "127.0.0.1" {
		if p := origin.Port(); p != strconv.Itoa(port) {
			origin.Host = fmt.Sprintf("%s:%d", h, port)
		}
	}

	if i, err := strconv.Atoi(origin.Port()); err == nil {
		port = i
	}

	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("could not open db connection: %w", err)
	}

	defer db.Close()

	if err = db.Ping(ctx); err != nil {
		return fmt.Errorf("could not ping to db: %w", err)
	}

	cockroach := cockroachpkg.New(db)

	if execSchema {
		if err := cockroach.Migrate(ctx); err != nil {
			return err
		}
	}

	natsConn, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("could not connect to NATS server: %w", err)
	}

	pubsub := &natspubsub.PubSub{Conn: natsConn}

	var sender mailing.Sender
	sendFrom := "no-reply@" + origin.Hostname()
	if resendAPIKey != "" {
		_ = logger.Log("mailing_implementation", "resend")
		sender = mailing.NewResend(sendFrom, resendAPIKey)
	} else if smtpUsername != "" && smtpPassword != "" {
		_ = logger.Log("mailing_implementation", "smtp")
		sender = mailing.NewSMTPSender(
			sendFrom,
			smtpHost,
			smtpPort,
			smtpUsername,
			smtpPassword,
		)
	} else {
		_ = logger.Log("mailing_implementation", "log")
		sender = mailing.NewLogSender(
			sendFrom,
			log.With(logger, "component", "mailing"),
		)
	}

	var store storage.Store
	s3Enabled := s3Endpoint != "" && s3AccessKey != "" && s3SecretKey != ""
	if s3Enabled {
		_ = logger.Log("storage_implementation", "s3")
		s3 := &s3storage.Store{
			Secure:     s3Secure,
			Endpoint:   s3Endpoint,
			Region:     s3Region,
			AccessKey:  s3AccessKey,
			SecretKey:  s3SecretKey,
			BucketList: []string{service.AvatarsBucket, service.CoversBucket, service.MediaBucket},
		}
		if err := s3.Setup(ctx); err != nil {
			return fmt.Errorf("could not setup S3 storage: %w", err)
		}

		store = s3
	} else {
		_ = logger.Log("storage_implementation", "os file system")
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get current working directory: %w", err)
		}

		store = &fsstorage.Store{Root: filepath.Join(wd, "web", "static", "img")}
	}

	svc := &service.Service{
		Logger:           logger,
		DB:               db,
		Cockroach:        cockroach,
		Sender:           sender,
		Origin:           origin,
		TokenKey:         tokenKey,
		PubSub:           pubsub,
		Store:            store,
		AvatarURLPrefix:  avatarURLPrefix,
		CoverURLPrefix:   coverURLPrefix,
		MediaURLPrefix:   mediaURLPrefix,
		DisabledDevLogin: disabledDevLogin,
		AllowedOrigins:   strings.Split(allowedOrigins, ","),
		VAPIDPrivateKey:  vapidPrivateKey,
		VAPIDPublicKey:   vapidPublicKey,
	}

	promHandler := promhttp.Handler()

	var oauthProviders []httptransport.OauthProvider
	if githubClientID != "" && githubClientSecret != "" {
		oauthProviders = append(oauthProviders, httptransport.OauthProvider{
			Name: "github",
			Config: &oauth2.Config{
				ClientID:     githubClientID,
				ClientSecret: githubClientSecret,
				RedirectURL:  origin.String() + "/api/github_auth/callback",
				Endpoint:     endpoints.GitHub,
				Scopes:       []string{"read:user", "user:email"},
			},
			FetchUser: httptransport.GithubUserFetcher,
		})
	}
	if googleClientID != "" && googleClientSecret != "" {
		provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			return fmt.Errorf("setup google oidc: %w", err)
		}

		oauthProviders = append(oauthProviders, httptransport.OauthProvider{
			Name: "google",
			Config: &oauth2.Config{
				ClientID:     googleClientID,
				ClientSecret: googleClientSecret,
				RedirectURL:  origin.String() + "/api/google_auth/callback",
				Endpoint:     provider.Endpoint(),
				Scopes: []string{
					oidc.ScopeOpenID,
					"profile",
					"email",
				},
			},
			IDTokenVerifier: provider.Verifier(&oidc.Config{
				ClientID: googleClientID,
			}),
		})
	}
	cookieCodec := securecookie.New(
		[]byte(cookieHashKey),
		[]byte(cookieBlockKey),
	)
	h := httptransport.New(svc, oauthProviders, origin, log.With(logger, "component", "http"), store, cookieCodec, promHandler, embedStaticFiles)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           h,
		ReadHeaderTimeout: time.Second * 10,
		ReadTimeout:       time.Second * 30,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errs := make(chan error, 1)
	go func() {
		<-ctx.Done()
		fmt.Println()

		_ = logger.Log("message", "gracefully shutting down")
		ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), time.Second*5)
		defer cancelShutdown()
		if err := server.Shutdown(ctxShutdown); err != nil {
			errs <- fmt.Errorf("could not shutdown server: %w", err)
		}

		errs <- nil
	}()

	_ = logger.Log("message", "accepting connections", "port", port)
	_ = logger.Log("message", "starting server", "origin", origin)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		close(errs)
		return fmt.Errorf("could not listen and serve: %w", err)
	}

	return <-errs
}

func env(key, fallbackValue string) string {
	s, ok := os.LookupEnv(key)
	if !ok {
		return fallbackValue
	}
	return s
}
