package uploads

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

// Handler exposes the chat-attachment endpoints.
//
// Routes:
//
//	POST /api/uploads     — multipart upload (auth required). Returns
//	                         { id, url, mime_type, size, original_name }.
//	GET  /api/uploads/:id  — download (signature-gated, no JWT).
//
// MountAuth wires only the POST route on a router that's already
// behind RequireAuth. MountPublic wires the GET route on the bare
// app — it's intentionally unauthenticated because the agent's MCP
// tool fetches it and doesn't have a user JWT.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// MountAuth registers POST /uploads on `r`. Caller is expected to
// apply auth middleware to the parent group.
func (h *Handler) MountAuth(r fiber.Router) {
	r.Post("/uploads", h.create)
}

// MountPublic registers GET /api/uploads/:id on `app`. The route
// validates a signed URL — there's no JWT — so it must NOT live
// behind RequireAuth.
func (h *Handler) MountPublic(app fiber.Router) {
	app.Get("/api/uploads/:id", h.download)
}

type createResponse struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	OriginalName string `json:"original_name,omitempty"`
}

func (h *Handler) create(c *fiber.Ctx) error {
	userID := apperrors.UserID(c)
	if userID == "" {
		return apperrors.ErrUnauthorized
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return apperrors.New(http.StatusBadRequest, "expected multipart field 'file'")
	}
	if fh.Size > h.svc.cfg.MaxBytes {
		return apperrors.New(http.StatusRequestEntityTooLarge, "file too large (cap 10MB)")
	}
	mime := fh.Header.Get("Content-Type")
	if mime == "" {
		// Multipart parts often arrive without an explicit Content-Type.
		// We don't try to sniff here — the agent's downstream tool
		// (e.g. reddit_submit_image) sniffs the body itself if needed.
		mime = "application/octet-stream"
	}

	src, err := fh.Open()
	if err != nil {
		return apperrors.New(http.StatusInternalServerError, "failed to open upload")
	}
	defer src.Close()

	up, err := h.svc.Save(userID, fh.Filename, mime, src)
	switch {
	case err == nil:
	case errors.Is(err, ErrTooLarge):
		return apperrors.New(http.StatusRequestEntityTooLarge, "file too large (cap 10MB)")
	default:
		return apperrors.New(http.StatusBadRequest, err.Error())
	}

	return c.Status(http.StatusCreated).JSON(createResponse{
		ID:           up.ID,
		URL:          h.svc.SignedURL(up.ID, up.CreatedAt),
		MimeType:     up.MimeType,
		Size:         up.Size,
		OriginalName: up.OriginalName,
	})
}

func (h *Handler) download(c *fiber.Ctx) error {
	id := c.Params("id")
	exp := c.Query("exp")
	sig := c.Query("sig")
	f, meta, err := h.svc.VerifyAndOpen(id, exp, sig, timeNow())
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		return apperrors.ErrNotFound
	case errors.Is(err, ErrExpired):
		return apperrors.New(http.StatusGone, "signed URL expired")
	case errors.Is(err, ErrBadSignature):
		// Generic 404 — don't tell the caller whether the id exists or
		// the signature is wrong.
		return apperrors.ErrNotFound
	default:
		return err
	}
	defer f.Close()

	c.Set("Content-Type", meta.MimeType)
	c.Set("Cache-Control", "private, max-age=300")
	if meta.OriginalName != "" {
		c.Set("Content-Disposition", "inline; filename="+quoteFilename(meta.OriginalName))
	}
	return c.SendStream(f, int(meta.Size))
}

// timeNow is a seam for tests that need to advance the clock without
// reaching into the service. The handler-level path is small enough
// that a direct time.Now() would also be fine — kept as a function
// for symmetry with VerifyAndOpen which takes a `now` arg explicitly.
var timeNow = func() time.Time { return time.Now() }

// quoteFilename produces an RFC 2616-safe Content-Disposition value.
// We don't bother with the full RFC 5987 (UTF-8 encoded) form — chat
// attachments are typically short ASCII names from the picker and the
// browser tolerates an unquoted ASCII fallback when the encoded
// variant is missing. Anything weirder, we strip.
func quoteFilename(name string) string {
	cleaned := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '"' || ch == '\\' || ch < 0x20 || ch >= 0x7f {
			continue
		}
		cleaned = append(cleaned, ch)
	}
	return `"` + strings.TrimSpace(string(cleaned)) + `"`
}
