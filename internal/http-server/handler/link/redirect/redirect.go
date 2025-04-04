package redirect

import (
	"encoding/json"
	"net/http"

	"github.com/mssola/user_agent"

	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	resp "backend/internal/lib/api/response"
)

// LinkGetter is an interface for getting link by alias.

//go:generate go run github.com/vektra/mockery/v2 --name=LinkGetter
type LinkGetter interface {
	GetLink(alias string, country, device, browser string) (string, error)
}

func New(log *slog.Logger, linkGetter LinkGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.link.redirect.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		alias := chi.URLParam(r, "minilink")
		if alias == "" {
			log.Info("minilink is empty")

			render.JSON(w, r, resp.Error("invalid request"))

			return
		}

		ua := user_agent.New(r.UserAgent())
		browser, _ := ua.Browser()
		isMobile := ua.Mobile()

		device := "desktop"
		if isMobile {
			device = "mobile"
		}

		ip := getIP(r)
		country, _ := getCountry(ip)

		resLink, err := linkGetter.GetLink(alias, country, device, browser)
		if err != nil && err.Error() == "repository.GetLink: link not found" {
			log.Info("link not found", "alias", alias)

			render.JSON(w, r, resp.Error("not found"))

			return
		}
		if err != nil {
			log.Error("failed to get link", slog.String("error", err.Error()))

			render.JSON(w, r, resp.Error("internal error"))

			return
		}

		log.Info("got link", slog.String("link", resLink))

		http.Redirect(w, r, resLink, http.StatusFound)
	}
}

func getIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func getCountry(ip string) (string, error) {
	resp, err := http.Get("http://ip-api.com/json/" + ip)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var data struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	return data.Country, nil
}
