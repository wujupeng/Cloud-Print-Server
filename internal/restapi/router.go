package restapi

import (
	"github.com/go-chi/chi/v5"

	"github.com/cloud-print/server/internal/auth"
)

func RegisterRoutes(r chi.Router, handlers *Handlers) {
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", handlers.Login)
			r.Post("/register", handlers.Register)
		})

		r.Group(func(r chi.Router) {
			r.Use(auth.JWTMiddleware(handlers.jwtMgr))

			r.Get("/devices", handlers.ListDevices)

			r.Route("/documents", func(r chi.Router) {
				r.Post("/upload", handlers.UploadDocument)
				r.Get("/{id}/download", handlers.DownloadDocument)
			})

			r.Route("/tasks", func(r chi.Router) {
				r.Post("/", handlers.CreateTask)
				r.Get("/", handlers.ListTasks)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", handlers.GetTask)
					r.Delete("/", handlers.CancelTask)
				})
			})

			r.Get("/events", handlers.Events)
		})
	})
}
