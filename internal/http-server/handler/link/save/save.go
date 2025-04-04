package save

import (
	"errors"
	"io"
	"net/http"

	"log/slog"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"

	resp "backend/internal/lib/api/response"
)

type Request struct {
	Link    string `json:"link" validate:"required,url"`
	ShortID string `json:"short_id"`
}

type Response struct {
	resp.Response
	ShortID string `json:"short_id,omitempty"`
}

const shortIDLength = 6

type LinkSaver interface {
	SaveLink(linkToSave string, shortID string) error
}

func New(log *slog.Logger, linkSaver LinkSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.link.save.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request

		err := render.DecodeJSON(r.Body, &req)
		if errors.Is(err, io.EOF) {
			log.Error("request body is empty")

			render.JSON(w, r, resp.Error("empty request"))

			return
		}
		if err != nil {
			log.Error("failed to decode request body", err)

			render.JSON(w, r, resp.Error("failed to decode request"))

			return
		}

		log.Info("request body decoded", slog.Any("request", req))

		validate := validator.New()
		if err := validate.Struct(req); err != nil {
			validateErr := err.(validator.ValidationErrors)

			log.Error("invalid request", err)

			render.JSON(w, r, resp.ValidationError(validateErr))

			return
		}

		shortID := req.ShortID
		if shortID == "" {
			render.JSON(w, r, resp.Error("shortID cannot be empty"))
			return
		}

		err = linkSaver.SaveLink(req.Link, shortID)
		if err != nil && err.Error() == "repository.SaveLink: short id already exists" {
			log.Info("shortID already exists", slog.String("shortID", shortID))

			render.JSON(w, r, resp.Error("shortID already exists"))

			return
		}
		if err != nil {
			log.Error("failed to add link", err)

			render.JSON(w, r, resp.Error("failed to add link"))

			return
		}

		log.Info("link added")

		responseOK(w, r, shortID)
	}
}

func responseOK(w http.ResponseWriter, r *http.Request, shortID string) {
	render.JSON(w, r, Response{
		Response: resp.OK(),
		ShortID:  shortID,
	})
}
