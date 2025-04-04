package statistic

import (
	resp "backend/internal/lib/api/response"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	"backend/internal/repository"
)

//go:generate go run github.com/vektra/mockery/v2 --name=StatisticGetter
type StatisticGetter interface {
	GetStatistic(shortID string) (*repository.StatisticResponse, error)
}

func New(log *slog.Logger, statisticGetter StatisticGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.statistic.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		shortID := chi.URLParam(r, "short_id")
		if shortID == "" {
			log.Info("short_id is empty")

			render.JSON(w, r, resp.Error("invalid request"))

			return
		}

		statistic, err := statisticGetter.GetStatistic(shortID)
		if err != nil {
			log.Error("failed to get statistic", slog.String("error", err.Error()))

			render.JSON(w, r, resp.Error("not found"))

			return
		}

		render.JSON(w, r, statistic)
	}
}
