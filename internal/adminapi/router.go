package adminapi

import (
	"github.com/go-chi/chi/v5"

	"github.com/cloud-print/server/internal/auth"
)

func RegisterAdminRoutes(r chi.Router, handlers *AdminHandlers) {
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(auth.JWTMiddleware(handlers.jwtMgr))
		r.Use(auth.AdminOnlyMiddleware)

		r.Route("/users", func(r chi.Router) {
			r.Get("/", handlers.ListUsers)
			r.Post("/", handlers.CreateUser)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", handlers.UpdateUser)
				r.Delete("/", handlers.DeleteUser)
			})
		})

		r.Route("/factories", func(r chi.Router) {
			r.Get("/", handlers.ListFactories)
			r.Post("/", handlers.CreateFactory)
		})

		r.Route("/agents", func(r chi.Router) {
			r.Get("/", handlers.ListAgents)
			r.Post("/", handlers.RegisterAgent)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/credentials", handlers.GetAgentCredentials)
				r.Post("/config-update", handlers.ConfigUpdate)
			})
		})

		r.Route("/devices", func(r chi.Router) {
			r.Get("/", handlers.ListAllDevices)
			r.Post("/", handlers.AddDevice)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", handlers.UpdateDevice)
				r.Delete("/", handlers.DeleteDevice)
			})
		})

		r.Get("/audit-logs", handlers.ListAuditLogs)
	})
}