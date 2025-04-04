package statistic

import (
	resp "backend/internal/lib/api/response"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"

	"backend/internal/repository"
)

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
			log.Error("failed to get statistic", err)

			render.JSON(w, r, resp.Error("not found"))

			return
		}

		render.JSON(w, r, statistic)
	}
}
