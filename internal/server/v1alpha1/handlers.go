package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"atomys.codes/webhooked/internal/config"
	"atomys.codes/webhooked/pkg/formatting"
)

// Server is the server instance for the v1alpha1 version
// it will be used to handle the webhook call and store the data
// on the configured storages for the current spec
type Server struct {
	// config is the current configuration of the server
	config *config.Configuration
	// webhookService is the function that will be called to process the webhook
	webhookService func(s *Server, spec *config.WebhookSpec, r *http.Request) error
	// logger is the logger used by the server
	logger zerolog.Logger
}

// errSecurityFailed is returned when security check failed for a webhook call
var errSecurityFailed = errors.New("security check failed")

// errRequestBodyMissing is returned when the request body is missing
var errRequestBodyMissing = errors.New("request body is missing")

// NewServer creates a new server instance for the v1alpha1 version
func NewServer() *Server {
	var s = &Server{
		config:         config.Current(),
		webhookService: webhookService,
	}

	s.logger = log.With().Str("apiVersion", s.Version()).Logger().Output(zerolog.ConsoleWriter{Out: os.Stderr})
	return s
}

// Version returns the current version of the API
func (s *Server) Version() string {
	return "v1alpha1"
}

// WebhookHandler is the handler who will process the webhook call
// it will call the webhook service function with the current configuration
// and the request object. If an error is returned, it will be returned to the client
// otherwise, it will return a 200 OK response
func (s *Server) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.APIVersion != s.Version() {
			s.logger.Error().Msgf("Configuration %s don't match with the API version %s", s.config.APIVersion, s.Version())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		endpoint := strings.ReplaceAll(r.URL.Path, "/"+s.Version(), "")
		spec, err := s.config.GetSpecByEndpoint(endpoint)
		if err != nil {
			log.Warn().Err(err).Msgf("No spec found for %s endpoint", endpoint)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if err := s.webhookService(s, spec, r); err != nil {
			switch err {
			case errSecurityFailed:
				w.WriteHeader(http.StatusForbidden)
				return
			default:
				s.logger.Error().Err(err).Msg("Error during webhook processing")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		s.logger.Debug().Str("entry", spec.Name).Msg("Webhook processed successfully")
	}
}

// webhookService is the function that will be called to process the webhook call
// it will call the security pipeline if configured and store data on each configured
// storages
func webhookService(s *Server, spec *config.WebhookSpec, r *http.Request) (err error) {
	if spec == nil {
		return config.ErrSpecNotFound
	}

	if r.Body == nil {
		return errRequestBodyMissing
	}
	defer r.Body.Close()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}

	if spec.HasSecurity() {
		if err := s.runSecurity(spec, r, data); err != nil {
			return err
		}
	}

	for _, storage := range spec.Storage {
		str, err := formatting.
			NewTemplateData(storage.Formatting.Template).
			WithRequest(r).
			WithPayload(data).
			WithData("Spec", spec).
			WithData("Storage", storage).
			WithData("Config", config.Current()).
			Render()
		if err != nil {
			return err
		}

		log.Debug().Msgf("store following data: %+v", str)
		if err := storage.Client.Push(str); err != nil {
			return err
		}
		log.Debug().Str("storage", storage.Client.Name()).Msgf("stored successfully")
	}

	return err
}

// runSecurity will run the security pipeline for the current webhook call
// it will check if the request is authorized by the security configuration of
// the current spec, if the request is not authorized, it will return an error
func (s *Server) runSecurity(spec *config.WebhookSpec, r *http.Request, body []byte) error {
	if spec == nil {
		return config.ErrSpecNotFound
	}

	if spec.SecurityPipeline == nil {
		return errors.New("no pipeline to run. security is not configured")
	}

	pipeline := spec.SecurityPipeline.DeepCopy()
	pipeline.
		WithInput("request", r).
		WithInput("payload", string(body)).
		WantResult(true).
		Run()

	log.Debug().Msgf("security pipeline result: %t", pipeline.CheckResult())
	if !pipeline.CheckResult() {
		return errSecurityFailed
	}
	return nil
}
