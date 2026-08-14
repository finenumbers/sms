package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/campaigns"
	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	adminhttp "finenumbers/sms/internal/http/admin"
	clienthttp "finenumbers/sms/internal/http/client"
	ingresshttp "finenumbers/sms/internal/http/ingress"
	publicapi "finenumbers/sms/internal/http/publicapi"
	smschttp "finenumbers/sms/internal/http/smsc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/inventory"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/messaging"
	"finenumbers/sms/internal/metrics"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/ratelimit"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/smsc"
	"finenumbers/sms/internal/webhooks"
)

type ReadyChecker interface {
	Ready(r *http.Request) error
}

type Deps struct {
	Log       *slog.Logger
	Cfg       config.Config
	Store     *db.Store
	Ident     *identity.Service
	Audit     *audit.Logger
	Limiter   *ratelimit.Limiter
	Settings  *settings.Service
	Runexis   *runexis.Client
	Inventory *inventory.Service
	Messages  *messaging.Service
	Campaigns *campaigns.Service
	Keys      *apikeys.Service
	Billing   *billing.Service
	Ready     ReadyChecker
	Ops       *ops.Logger
	SMSC      *smsc.Provider
	SMSCCache *smsc.BalanceCache
	Lookup    *lookup.Worker
	LookupSvc *lookup.Service
	Webhooks  *webhooks.Service
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Log, d.Ops))
	r.Use(RestrictSurface(d.Cfg))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	r.Handle("/metrics", metrics.Handler())
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if err := d.Store.Ping(req.Context()); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "error": "postgres"})
			return
		}
		if d.Ready != nil {
			if err := d.Ready.Ready(req); err != nil {
				httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "error": err.Error()})
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})

	adminH := &adminhttp.Handlers{
		Log:          d.Log,
		Cfg:          d.Cfg,
		Ident:        d.Ident,
		Audit:        d.Audit,
		Limiter:      d.Limiter,
		Settings:     d.Settings,
		Runexis:      d.Runexis,
		Inventory:    d.Inventory,
		Store:        d.Store,
		Keys:         d.Keys,
		Billing:      d.Billing,
		Lookup:       d.LookupSvc,
		LookupWorker: d.Lookup,
		SMSC:         d.SMSC,
		SMSCCache:    d.SMSCCache,
	}
	clientH := &clienthttp.Handlers{
		Log:       d.Log,
		Cfg:       d.Cfg,
		Ident:     d.Ident,
		Audit:     d.Audit,
		Limiter:   d.Limiter,
		Messages:  d.Messages,
		Campaigns: d.Campaigns,
		Settings:  d.Settings,
		Keys:      d.Keys,
		Inventory: d.Inventory,
		Billing:   d.Billing,
		Store:     d.Store,
		Lookup:    d.LookupSvc,
		Webhooks:  d.Webhooks,
	}
	publicH := &publicapi.Handlers{
		Log:      d.Log,
		Store:    d.Store,
		Messages: d.Messages,
		Lookup:   d.LookupSvc,
	}
	ingH := &ingresshttp.Handlers{
		Log:     d.Log,
		Events:  d.Store.Queries,
		Tokens:  d.Settings,
		Limiter: d.Limiter,
		Ops:     d.Ops,
	}

	r.Route("/admin/v1", func(r chi.Router) {
		r.Use(httpx.CSRF)
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"surface": "admin"})
		})
		r.Post("/auth/login", adminH.Login)
		r.Group(func(r chi.Router) {
			r.Use(RequireAudience(d.Ident, d.Cfg, sqlcdb.SessionAudienceAdmin))
			r.Post("/auth/logout", adminH.Logout)
			r.Get("/auth/me", adminH.Me)
			r.Post("/clients", adminH.CreateClient)
			r.Get("/clients", adminH.ListClients)
			r.Get("/clients/{clientID}", adminH.GetClient)
			r.Patch("/clients/{clientID}", adminH.PatchClient)
			r.Post("/clients/{clientID}/suspend", adminH.SuspendClient)
			r.Post("/clients/{clientID}/activate", adminH.ActivateClient)
			r.Delete("/clients/{clientID}", adminH.DeleteClient)
			r.Post("/clients/{clientID}/owner/password", adminH.ResetOwnerPassword)
			r.Get("/numbers", adminH.ListNumbers)
			r.Post("/numbers/upload", adminH.UploadNumbers)
			r.Post("/numbers/sync", adminH.SyncNumbers)
			r.Get("/numbers/{numberID}", adminH.GetNumber)
			r.Patch("/numbers/{numberID}", adminH.PatchNumber)
			r.Post("/numbers/{numberID}/assign", adminH.AssignNumber)
			r.Post("/numbers/{numberID}/unassign", adminH.UnassignNumber)
			r.Get("/settings", adminH.GetSettings)
			r.Patch("/settings", adminH.PatchSettings)
			r.Post("/settings/runexis/test", adminH.TestRunexis)
			r.Post("/settings/runexis/callbacks", adminH.RegisterCallbacks)
			r.Get("/callbacks", adminH.ListCallbacks)
			r.Get("/callbacks/{callbackID}", adminH.GetCallback)
			r.Get("/logs", adminH.ListLogs)
			r.Get("/logs/{logID}", adminH.GetLog)
			r.Get("/clients/{clientID}/api-keys", adminH.ListAPIKeys)
			r.Post("/clients/{clientID}/api-keys", adminH.CreateAPIKey)
			r.Post("/clients/{clientID}/api-keys/{keyID}/revoke", adminH.RevokeAPIKey)
			r.Get("/billing/overview", adminH.BillingOverview)
			r.Get("/billing/ledger", adminH.PlatformLedger)
			r.Get("/clients/{clientID}/billing", adminH.GetClientBilling)
			r.Post("/clients/{clientID}/billing/topup", adminH.TopUpClient)
			r.Post("/clients/{clientID}/billing/adjust", adminH.AdjustClient)
			r.Post("/clients/{clientID}/tariff", adminH.AssignClientTariff)
			r.Delete("/clients/{clientID}/tariff/{product}", adminH.UnassignClientTariff)
			r.Get("/tariffs", adminH.ListTariffs)
			r.Post("/tariffs", adminH.CreateTariff)
			r.Patch("/tariffs/{tariffID}", adminH.PatchTariff)
			r.Get("/lookups/jobs", adminH.LookupListJobs)
			r.Get("/lookups/jobs/{jobID}", adminH.LookupGetJob)
			r.Get("/lookups/jobs/{jobID}/items", adminH.LookupListItems)
			r.Get("/lookups/jobs/{jobID}/export", adminH.LookupExportJob)
			r.Post("/lookups/jobs/{jobID}/finalize", adminH.LookupFinalizeJob)
			r.Get("/lookups/monitoring", adminH.LookupMonitoring)
			r.Post("/provider/smsc/estimate-cost", adminH.SMSCEstimateCost)
			r.Get("/provider/smsc/balance", adminH.SMSCBalance)
			r.Post("/provider/smsc/connectivity-test", adminH.SMSCConnectivityTest)
		})
	})

	r.Route("/client/v1", func(r chi.Router) {
		r.Use(httpx.CSRF)
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"surface": "client"})
		})
		r.Post("/auth/login", clientH.Login)
		r.Group(func(r chi.Router) {
			r.Use(RequireAudience(d.Ident, d.Cfg, sqlcdb.SessionAudienceClient))
			r.Post("/auth/logout", clientH.Logout)
			r.Get("/auth/me", clientH.Me)
			r.Post("/messages", clientH.SendMessage)
			r.Get("/messages", clientH.ListMessages)
			r.Get("/messages/{messageID}", clientH.GetMessage)
			r.Post("/campaigns", clientH.CreateCampaign)
			r.Get("/campaigns", clientH.ListCampaigns)
			r.Get("/campaigns/{campaignID}", clientH.GetCampaign)
			r.Patch("/campaigns/{campaignID}", clientH.PatchCampaign)
			r.Delete("/campaigns/{campaignID}", clientH.DeleteCampaign)
			r.Post("/campaigns/{campaignID}/recipients", clientH.AddRecipients)
			r.Post("/campaigns/{campaignID}/recipients/upload", clientH.UploadRecipients)
			r.Get("/campaigns/{campaignID}/recipients", clientH.ListRecipients)
			r.Post("/campaigns/{campaignID}/start", clientH.StartCampaign)
			r.Post("/campaigns/{campaignID}/cancel", clientH.CancelCampaign)
			r.Get("/api-keys", clientH.ListAPIKeys)
			r.Get("/numbers", clientH.ListNumbers)
			r.Get("/billing/balance", clientH.GetBalance)
			r.Get("/billing/ledger", clientH.GetLedger)
			r.Get("/billing/tariff", clientH.GetTariff)
			r.Get("/billing/stats", clientH.GetStats)
			r.Post("/billing/estimate", clientH.Estimate)
			r.Get("/campaigns/{campaignID}/estimate", clientH.EstimateCampaign)
			r.Post("/lookups/estimate", clientH.LookupEstimate)
			r.Post("/lookups/checks", clientH.LookupCreateCheck)
			r.Post("/lookups/jobs", clientH.LookupCreateJob)
			r.Get("/lookups/jobs", clientH.LookupListJobs)
			r.Get("/lookups/jobs/{jobID}", clientH.LookupGetJob)
			r.Get("/lookups/jobs/{jobID}/items", clientH.LookupListItems)
			r.Get("/lookups/jobs/{jobID}/export", clientH.LookupExportJob)
			r.Post("/lookups/csv-previews", clientH.LookupCreateCSVPreview)
			r.Get("/lookups/csv-previews/{previewID}", clientH.LookupGetCSVPreview)
			r.Post("/lookups/csv-previews/{previewID}/estimate", clientH.LookupEstimateCSVPreview)
			r.Post("/lookups/csv-previews/{previewID}/submit", clientH.LookupSubmitCSVPreview)
			r.Delete("/lookups/csv-previews/{previewID}", clientH.LookupDeleteCSVPreview)
			r.Get("/webhooks", clientH.ListWebhooks)
			r.Post("/webhooks", clientH.CreateWebhook)
			r.Get("/webhooks/deliveries", clientH.ListWebhookDeliveries)
			r.Get("/webhooks/{webhookID}", clientH.GetWebhook)
			r.Patch("/webhooks/{webhookID}", clientH.PatchWebhook)
			r.Delete("/webhooks/{webhookID}", clientH.DeleteWebhook)
			r.Post("/webhooks/{webhookID}/rotate-secret", clientH.RotateWebhookSecret)
			r.Get("/webhooks/{webhookID}/deliveries", clientH.ListWebhookDeliveries)
		})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(httpx.CORS(d.Cfg.CORSAllowOrigins))
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"surface": "public"})
		})
		r.Group(func(r chi.Router) {
			r.Use(RequireAPIKey(d.Keys, d.Log))
			r.Use(PublicAPIRateLimit(d.Limiter, d.Log))
			r.With(RequireScope(apikeys.ScopeSend)).Post("/messages", publicH.SendMessage)
			r.With(RequireScope(apikeys.ScopeRead)).Get("/messages", clientH.ListMessages)
			r.With(RequireScope(apikeys.ScopeRead)).Get("/messages/{messageID}", clientH.GetMessage)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Post("/campaigns", clientH.CreateCampaign)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Get("/campaigns", clientH.ListCampaigns)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Get("/campaigns/{campaignID}", clientH.GetCampaign)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Patch("/campaigns/{campaignID}", clientH.PatchCampaign)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Delete("/campaigns/{campaignID}", clientH.DeleteCampaign)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Post("/campaigns/{campaignID}/recipients", clientH.AddRecipients)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Post("/campaigns/{campaignID}/recipients/upload", clientH.UploadRecipients)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Get("/campaigns/{campaignID}/recipients", clientH.ListRecipients)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Post("/campaigns/{campaignID}/start", clientH.StartCampaign)
			r.With(RequireScope(apikeys.ScopeCampaigns)).Post("/campaigns/{campaignID}/cancel", clientH.CancelCampaign)
			r.With(RequireScope(apikeys.ScopeRead)).Get("/balance", clientH.GetBalance)
			r.With(RequireScope(apikeys.ScopeRead)).Get("/usage", clientH.GetStats)
			r.With(RequireScope(apikeys.ScopeLookupWrite)).Post("/checks", publicH.CreateCheck)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/checks", publicH.ListChecks)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/checks/{checkID}", publicH.GetCheck)
			r.With(RequireScope(apikeys.ScopeLookupWrite)).Post("/jobs", publicH.CreateJob)
			r.With(RequireScope(apikeys.ScopeLookupWrite)).Post("/jobs/csv", publicH.CreateJobCSV)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/jobs", publicH.ListJobs)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/jobs/{jobID}", publicH.GetJob)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/jobs/{jobID}/items", publicH.ListJobItems)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/webhooks", clientH.ListWebhooks)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/webhooks/deliveries", clientH.ListWebhookDeliveries)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/webhooks/{webhookID}", clientH.GetWebhook)
			r.With(RequireScope(apikeys.ScopeLookupRead)).Get("/webhooks/{webhookID}/deliveries", clientH.ListWebhookDeliveries)
		})
	})

	r.HandleFunc("/internal/runexis/{kind}/{token}", ingH.Capture)
	smscH := &smschttp.Handlers{
		Log:      d.Log,
		Provider: d.SMSC,
		Lookup:   d.Lookup,
		Limiter:  d.Limiter,
		Ops:      d.Ops,
	}
	r.Get("/internal/smsc/callback", smscH.Callback)
	r.Post("/internal/smsc/callback", smscH.Callback)
	r.NotFound(SPA(d.Cfg).ServeHTTP)

	return r
}
