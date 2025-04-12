package redirect

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

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
		log.Debug("Detected IP", slog.String("ip", ip))

		country, err := getCountry(ip)
		if err != nil {
			log.Error("Failed to get country", slog.String("error", err.Error()))
			country = "unknown"
		}

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
	ip := r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		ip = strings.TrimSpace(ips[0])
		if ip != "" {
			return ip
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
}

func getCountry(ip string) (string, error) {
	if isPrivateIP(ip) {
		return "local", nil
	}

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

func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	if parsedIP.IsPrivate() {
		return true
	}

	// Проверяем локальные адреса
	if parsedIP.IsLoopback() {
		return true
	}

	return false
}
