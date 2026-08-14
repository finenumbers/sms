package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/campaigns"
	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/db"
	"finenumbers/sms/internal/httpserver"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/inventory"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/messaging"
	"finenumbers/sms/internal/metrics"
	"finenumbers/sms/internal/migrate"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/outbox"
	"finenumbers/sms/internal/ratelimit"
	"finenumbers/sms/internal/redisx"
	"finenumbers/sms/internal/retention"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/secret"
	"finenumbers/sms/internal/seed"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/smsc"
	"finenumbers/sms/internal/webhooks"
)

func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	switch cfg.Mode {
	case config.ModeMigrate:
		return migrate.Up(cfg.DatabaseURL)
	case config.ModeAPI:
		return runAPI(ctx, cfg, log, false)
	case config.ModeWorker:
		return runWorker(ctx, cfg, log)
	case config.ModeAll:
		return runAPI(ctx, cfg, log, true)
	default:
		return fmt.Errorf("unknown mode %q", cfg.Mode)
	}
}

func runAPI(ctx context.Context, cfg config.Config, log *slog.Logger, withWorker bool) error {
	if err := migrate.Up(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := seed.Admin(ctx, log, store, cfg); err != nil {
		return err
	}

	rdb, err := redisx.Connect(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	bill := billing.New(store, log)
	ident := identity.New(store, cfg.SessionTTL)
	keys := apikeys.New(store, cfg.APIKeyPepperValue())
	opsLog := ops.New(store.Queries, log)
	aud := audit.New(store.Queries, log, opsLog)
	limiter := ratelimit.New(rdb.Cmdable())
	settingsSvc, rx, err := buildProvider(cfg, store, rdb, log, opsLog)
	if err != nil {
		return err
	}
	inv := inventory.New(store, settingsSvc, rx)
	msg := messaging.New(store, settingsSvc, bill)
	camp := campaigns.New(store, settingsSvc, bill)
	sendW := outbox.NewWorker(store, rx, limiter, settingsSvc, bill, log, opsLog)
	dirW := inventory.NewDirectionWorker(store, rx, log, opsLog)
	fanW := campaigns.NewWorker(store, settingsSvc, msg, log)
	cbW := ingress.NewWorker(store, bill, log, opsLog)
	retW := retention.NewWorker(store, settingsSvc, log)
	smscProv := newSMSCProvider(settingsSvc, store, log)
	smscCache := smsc.NewBalanceCache(rdb.Cmdable())
	lookupW, lookupSvc, hooks := newLookup(store, bill, smscProv, settingsSvc, limiter, log, smscCache)
	metrics.RegisterStore(store.Queries)
	metrics.RegisterLookupRuntime(lookupRuntime{settings: settingsSvc, smsc: smscProv, cache: smscCache})

	stopWorker := func() {}
	waitWorker := func() {}
	if withWorker {
		var workerCtx context.Context
		workerCtx, stopWorker = context.WithCancel(ctx)
		waitWorker = startWorkerLoop(workerCtx, log, dirW, fanW, sendW, cbW, retW)
	}
	// Lookup Tick is never inside the SMS loop: a started SMSC call can take 3s.
	// ModeAPI still needs CSV parse / submit / poll. SKIP LOCKED is safe if
	// ModeAll or a separate worker also ticks. Child ctx so ListenAndServe
	// failure still stops the loops before store.Close.
	lookupCtx, stopLookup := context.WithCancel(ctx)
	waitLookup := startLookupSideLoops(lookupCtx, log, lookupW)
	defer func() {
		stopWorker()
		stopLookup()
		waitWorker()
		waitLookup()
	}()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewRouter(httpserver.Deps{
			Log:       log,
			Cfg:       cfg,
			Store:     store,
			Ident:     ident,
			Audit:     aud,
			Limiter:   limiter,
			Settings:  settingsSvc,
			Runexis:   rx,
			Inventory: inv,
			Messages:  msg,
			Campaigns: camp,
			Keys:      keys,
			Billing:   bill,
			Ready:     rdb,
			Ops:       opsLog,
			SMSC:      smscProv,
			SMSCCache: smscCache,
			Lookup:    lookupW,
			LookupSvc: lookupSvc,
			Webhooks:  hooks,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func runWorker(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	if err := migrate.Up(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	rdb, err := redisx.Connect(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	opsLog := ops.New(store.Queries, log)
	settingsSvc, rx, err := buildProvider(cfg, store, rdb, log, opsLog)
	if err != nil {
		return err
	}
	limiter := ratelimit.New(rdb.Cmdable())
	metrics.RegisterStore(store.Queries)
	smscProv := newSMSCProvider(settingsSvc, store, log)
	smscCache := smsc.NewBalanceCache(rdb.Cmdable())
	metrics.RegisterLookupRuntime(lookupRuntime{settings: settingsSvc, smsc: smscProv, cache: smscCache})
	log.Info("worker started")
	if cfg.MetricsAddr != "" {
		go serveMetrics(ctx, cfg.MetricsAddr, log)
	}
	bill := billing.New(store, log)
	lookupW, _, _ := newLookup(store, bill, smscProv, settingsSvc, limiter, log, smscCache)
	lookupCtx, stopLookup := context.WithCancel(ctx)
	waitLookup := startLookupSideLoops(lookupCtx, log, lookupW)
	defer func() {
		stopLookup()
		waitLookup()
	}()
	return runWorkerLoop(ctx, log, inventory.NewDirectionWorker(store, rx, log, opsLog), campaigns.NewWorker(store, settingsSvc, messaging.New(store, settingsSvc, bill), log), outbox.NewWorker(store, rx, limiter, settingsSvc, bill, log, opsLog), ingress.NewWorker(store, bill, log, opsLog), retention.NewWorker(store, settingsSvc, log))
}

type runexisCreds struct {
	svc *settings.Service
}

func (r runexisCreds) RunexisCredentials(ctx context.Context) (runexis.Credentials, error) {
	c, err := r.svc.RunexisCredentials(ctx)
	if err != nil {
		if errors.Is(err, settings.ErrNotConfigured) {
			return runexis.Credentials{}, runexis.ErrNotConfigured
		}
		return runexis.Credentials{}, err
	}
	return runexis.Credentials{Email: c.Email, Password: c.Password}, nil
}

func buildProvider(cfg config.Config, store *db.Store, rdb *redisx.Client, log *slog.Logger, opsLog *ops.Logger) (*settings.Service, *runexis.Client, error) {
	kr, err := secret.NewKeyring(cfg.AppMasterKey, cfg.AppMasterKeyPrev)
	if err != nil {
		return nil, nil, fmt.Errorf("master key: %w", err)
	}
	settingsSvc := settings.New(store.Queries, kr)
	rx := runexis.New(runexis.Options{
		BaseURL: cfg.RunexisBaseURL,
		Redis:   rdb.Cmdable(),
		Creds:   runexisCreds{svc: settingsSvc},
		Log:     log,
		Ops:     opsLog,
	})
	return settingsSvc, rx, nil
}

type smscSettingsSource struct {
	svc *settings.Service
}

func (s smscSettingsSource) SMSCConfig(ctx context.Context) (smsc.Config, error) {
	sec, err := s.svc.SMSCSecrets(ctx)
	if err != nil {
		return smsc.Config{}, err
	}
	return smsc.Load(smsc.Input{
		BaseURL:        sec.BaseURL,
		Login:          sec.Login,
		Password:       sec.Password,
		APIKey:         sec.APIKey,
		Currency:       sec.Currency,
		CallbackSecret: sec.CallbackSecret,
	}), nil
}

func newSMSCProvider(settingsSvc *settings.Service, store *db.Store, log *slog.Logger) *smsc.Provider {
	return smsc.New(smsc.Options{
		Source:      smscSettingsSource{svc: settingsSvc},
		Persistence: lookup.NewPersistence(store.Queries),
		Log:         log,
	})
}

func newLookup(store *db.Store, bill *billing.Service, smscProv *smsc.Provider, settingsSvc *settings.Service, limiter *ratelimit.Limiter, log *slog.Logger, cache *smsc.BalanceCache) (*lookup.Worker, *lookup.Service, *webhooks.Service) {
	svc := lookup.NewService(store, bill, settingsSvc, log)
	hooks := webhooks.New(store, settingsSvc.Keyring(), settingsSvc, log)
	svc.SetWebhooks(hooks)
	w := lookup.NewWorker(store, bill, lookup.NewGateway(smscProv), settingsSvc, limiter, log)
	w.SetService(svc)
	w.SetWebhooks(hooks)
	if smscProv != nil {
		w.SetBalanceRefresh(cache, func(ctx context.Context) (smsc.Balance, error) {
			return smscProv.Balance(ctx, "worker-balance-cache")
		})
	}
	return w, svc, hooks
}

type lookupRuntime struct {
	settings *settings.Service
	smsc     *smsc.Provider
	cache    *smsc.BalanceCache
}

func (r lookupRuntime) LookupEnabled(ctx context.Context) bool {
	if r.settings == nil {
		return false
	}
	view, err := r.settings.Get(ctx)
	if err != nil {
		return false
	}
	return view.LookupEnabled
}

func (r lookupRuntime) SMSCConfigured(_ context.Context) bool {
	return r.smsc != nil && r.smsc.Configured()
}

func (r lookupRuntime) SMSCBalance(ctx context.Context) (float64, bool) {
	if r.cache == nil {
		return 0, false
	}
	n, ok, err := r.cache.Read(ctx)
	if err != nil || !ok {
		return 0, false
	}
	return n, true
}

// SMSC call cap + persist after cancel + slack. Must finish before store.Close.
const lookupLoopDrain = 10 * time.Second

// Runexis HTTP client timeout is 20s; one in-flight send must finish before store.Close.
const workerLoopDrain = 22 * time.Second

func startWorkerLoop(ctx context.Context, log *slog.Logger, dirs *inventory.DirectionWorker, fanout *campaigns.Worker, send *outbox.Worker, callbacks *ingress.Worker, retain *retention.Worker) (wait func()) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := runWorkerLoop(ctx, log, dirs, fanout, send, callbacks, retain); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("worker loop", "err", err)
		}
	}()
	return func() {
		waitWithTimeout(log, "sms worker loop", workerLoopDrain, wg.Wait)
	}
}

func startLookupSideLoops(ctx context.Context, log *slog.Logger, lookups *lookup.Worker) (wait func()) {
	if lookups == nil {
		return func() {}
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if err := runLookupLoop(ctx, log, lookups); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("lookup loop", "err", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := runLookupCallbackLoop(ctx, log, lookups); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("lookup callback loop", "err", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := runSMSCBalanceLoop(ctx, log, lookups); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("smsc balance loop", "err", err)
		}
	}()
	return func() {
		waitWithTimeout(log, "lookup loops", lookupLoopDrain, wg.Wait)
	}
}

func waitWithTimeout(log *slog.Logger, name string, timeout time.Duration, wait func()) {
	if wait == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
		if log != nil {
			log.Warn("background loop drain timeout", "loop", name, "timeout", timeout)
		}
	}
}

func runLookupLoop(ctx context.Context, log *slog.Logger, lookups *lookup.Worker) error {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("lookup worker stopped")
			return nil
		case <-t.C:
			if n, err := lookups.Tick(ctx); err != nil {
				log.Error("lookup jobs", "err", err)
			} else if n > 0 {
				log.Info("lookup jobs processed", "n", n)
			}
		}
	}
}

func runLookupCallbackLoop(ctx context.Context, log *slog.Logger, lookups *lookup.Worker) error {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("lookup callback loop stopped")
			return nil
		case <-t.C:
			if n, err := lookups.DrainCallbacks(ctx); err != nil {
				log.Error("lookup callbacks", "err", err)
			} else if n > 0 {
				log.Info("lookup callbacks applied", "n", n)
			}
		}
	}
}

func runSMSCBalanceLoop(ctx context.Context, log *slog.Logger, lookups *lookup.Worker) error {
	if lookups == nil {
		return nil
	}
	lookups.RefreshSMSCBalance(ctx)
	t := time.NewTicker(lookup.BalanceRefreshEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("smsc balance loop stopped")
			return nil
		case <-t.C:
			lookups.RefreshSMSCBalance(ctx)
		}
	}
}

func runWorkerLoop(ctx context.Context, log *slog.Logger, dirs *inventory.DirectionWorker, fanout *campaigns.Worker, send *outbox.Worker, callbacks *ingress.Worker, retain *retention.Worker) error {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("worker stopped")
			return nil
		case <-t.C:
			if n, err := dirs.Tick(ctx); err != nil {
				log.Error("direction jobs", "err", err)
			} else if n > 0 {
				log.Info("direction jobs processed", "n", n)
			}
			if n, err := fanout.Tick(ctx); err != nil {
				log.Error("campaign fan-out", "err", err)
			} else if n > 0 {
				log.Info("campaign fan-out processed", "n", n)
			}
			if n, err := callbacks.Tick(ctx); err != nil {
				log.Error("callbacks", "err", err)
			} else if n > 0 {
				log.Info("callbacks processed", "n", n)
			}
			if n, err := send.Tick(ctx); err != nil {
				log.Error("send jobs", "err", err)
			} else if n > 0 {
				log.Info("send jobs processed", "n", n)
			}
			if err := retain.Tick(ctx); err != nil {
				log.Error("retention", "err", err)
			}
		}
	}
}

func serveMetrics(ctx context.Context, addr string, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Info("worker metrics", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("worker metrics", "err", err)
	}
}
