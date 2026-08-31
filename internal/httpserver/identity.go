package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"exchange-lab/internal/identity"
)

const sessionCookieName = "exchange_session"

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    int64         `json:"id"`
	Email string        `json:"email"`
	Role  identity.Role `json:"role"`
}

type identityResponse struct {
	Data userResponse `json:"data"`
}

type auditResponse struct {
	Data []identity.AuditEvent `json:"data"`
}

func registerIdentityRoutes(mux *http.ServeMux, service IdentityService, limiter LoginLimiter, secure bool, ttl time.Duration, logger *slog.Logger) {
	mux.HandleFunc("/v1/auth/register", postOnly(logger, sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
		credentials, ok := decodeCredentials(writer, request, logger)
		if !ok {
			return
		}
		user, err := service.Register(request.Context(), credentials.Email, credentials.Password)
		if err != nil {
			writeIdentityError(writer, request, logger, err)
			return
		}
		writeResponse(loggerForRequest(request, logger), writer, http.StatusCreated, identityResponse{Data: publicIdentityUser(user)})
	})))
	mux.HandleFunc("/v1/auth/login", postOnly(logger, sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
		credentials, ok := decodeCredentials(writer, request, logger)
		if !ok {
			return
		}
		decision, err := limiter.Allow(request.Context(), loginRateIdentity(request, credentials.Email))
		if err != nil {
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusServiceUnavailable, "security_state_unavailable", "authentication temporarily unavailable")
			return
		}
		if !decision.Allowed {
			writer.Header().Set("Retry-After", strconv.Itoa(max(1, int(decision.RetryAfter.Seconds()))))
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		token, session, err := service.Login(request.Context(), credentials.Email, credentials.Password)
		if err != nil {
			writeIdentityError(writer, request, logger, err)
			return
		}
		setSessionCookie(writer, token, session.ExpiresAt, secure, ttl)
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, identityResponse{Data: publicIdentityUser(session.User)})
	})))
	mux.HandleFunc("/v1/auth/refresh", postOnly(logger, sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
		token, ok := sessionToken(request)
		if !ok {
			writeIdentityError(writer, request, logger, identity.ErrUnauthenticated)
			return
		}
		replacement, session, err := service.Refresh(request.Context(), token)
		if err != nil {
			writeIdentityError(writer, request, logger, err)
			return
		}
		setSessionCookie(writer, replacement, session.ExpiresAt, secure, ttl)
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, identityResponse{Data: publicIdentityUser(session.User)})
	})))
	mux.HandleFunc("/v1/auth/logout", postOnly(logger, sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
		if token, ok := sessionToken(request); ok {
			if err := service.Logout(request.Context(), token); err != nil {
				writeIdentityError(writer, request, logger, err)
				return
			}
		}
		clearSessionCookie(writer, secure)
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, response{Status: "ok"})
	})))
	mux.HandleFunc("/v1/me", getOnly(logger, func(writer http.ResponseWriter, request *http.Request) {
		session, ok := authenticatedSession(writer, request, service, logger)
		if !ok {
			return
		}
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, identityResponse{Data: publicIdentityUser(session.User)})
	}))
	mux.HandleFunc("/v1/admin/audit-logs", getOnly(logger, func(writer http.ResponseWriter, request *http.Request) {
		session, ok := authenticatedSession(writer, request, service, logger)
		if !ok {
			return
		}
		limit := 100
		if value := request.URL.Query().Get("limit"); value != "" {
			var err error
			limit, err = strconv.Atoi(value)
			if err != nil {
				writeIdentityError(writer, request, logger, identity.ErrInvalidInput)
				return
			}
		}
		events, err := service.AuditLog(request.Context(), session.User, limit)
		if err != nil {
			writeIdentityError(writer, request, logger, err)
			return
		}
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, auditResponse{Data: events})
	}))
}

func decodeCredentials(writer http.ResponseWriter, request *http.Request, logger *slog.Logger) (credentialsRequest, bool) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4096)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var credentials credentialsRequest
	if err := decoder.Decode(&credentials); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusBadRequest, "invalid_request", "invalid request")
		return credentialsRequest{}, false
	}
	return credentials, true
}

func authenticatedSession(writer http.ResponseWriter, request *http.Request, service IdentityService, logger *slog.Logger) (identity.Session, bool) {
	token, ok := sessionToken(request)
	if !ok {
		writeIdentityError(writer, request, logger, identity.ErrUnauthenticated)
		return identity.Session{}, false
	}
	session, err := service.Authenticate(request.Context(), token)
	if err != nil {
		writeIdentityError(writer, request, logger, err)
		return identity.Session{}, false
	}
	return session, true
}

func sessionToken(request *http.Request) (string, bool) {
	cookie, err := request.Cookie(sessionCookieName)
	return cookieValue(cookie, err)
}

func cookieValue(cookie *http.Cookie, err error) (string, bool) {
	return cookieValueValid(cookie, err)
}

func cookieValueValid(cookie *http.Cookie, err error) (string, bool) {
	if err != nil || cookie == nil || len(cookie.Value) < 40 || len(cookie.Value) > 128 {
		return "", false
	}
	return cookie.Value, true
}

func setSessionCookie(writer http.ResponseWriter, token string, expires time.Time, secure bool, ttl time.Duration) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", Expires: expires.UTC(), MaxAge: int(ttl.Seconds()), Secure: secure, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func clearSessionCookie(writer http.ResponseWriter, secure bool) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0).UTC(), Secure: secure, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func publicIdentityUser(user identity.User) userResponse {
	return userResponse{ID: user.ID, Email: user.Email, Role: user.Role}
}

func writeIdentityError(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, err error) {
	requestLogger := loggerForRequest(request, logger)
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials), errors.Is(err, identity.ErrUnauthenticated):
		writeAPIError(requestLogger, writer, request, http.StatusUnauthorized, "unauthenticated", "authentication required")
	case errors.Is(err, identity.ErrForbidden):
		writeAPIError(requestLogger, writer, request, http.StatusForbidden, "forbidden", "permission denied")
	case errors.Is(err, identity.ErrConflict):
		writeAPIError(requestLogger, writer, request, http.StatusConflict, "conflict", "resource already exists")
	case errors.Is(err, identity.ErrInvalidInput):
		writeAPIError(requestLogger, writer, request, http.StatusBadRequest, "invalid_request", "invalid request")
	default:
		requestLogger.Error("identity operation failed")
		writeAPIError(requestLogger, writer, request, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func postOnly(logger *slog.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		next(writer, request)
	}
}

func sameOrigin(logger *slog.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		crossSite := request.Header.Get("Sec-Fetch-Site") == "cross-site"
		if crossSite || (origin != "" && origin != "http://"+request.Host && origin != "https://"+request.Host) {
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusForbidden, "forbidden", "permission denied")
			return
		}
		next(writer, request)
	}
}

func loginRateIdentity(request *http.Request, email string) string {
	value := strings.ToLower(strings.TrimSpace(email)) + "|" + request.RemoteAddr
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
