package webapp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const acquisitionCookie = "mouseion_acquisition"
const maxAcquisitionEntries = 40
const maxAcquisitionCookieBytes = 3072

type acquisitionEntryState struct {
	Connection string `json:"connection"`
	Language   string `json:"language"`
	EntryID    string `json:"entry_id"`
	Href       string `json:"href"`
	HrefDigest string `json:"href_digest,omitempty"`
	SourceID   string `json:"source_id"`
}

type acquisitionCookiePayload struct {
	OwnerID     string                  `json:"owner_id"`
	SessionHash string                  `json:"session_hash"`
	Entries     []acquisitionEntryState `json:"entries"`
}

func (h *Handler) acquisitionState(r *http.Request) []acquisitionEntryState {
	cookie, err := r.Cookie(acquisitionCookie)
	ownerID, sessionHash, ok := acquisitionBinding(r)
	if err != nil || cookie.Value == "" || !ok {
		return nil
	}
	return h.acquisitionStateFor(cookie.Value, ownerID, sessionHash)
}

func (h *Handler) acquisitionStateFor(value, ownerID, sessionHash string) []acquisitionEntryState {
	if value == "" || ownerID == "" || sessionHash == "" {
		return nil
	}
	encoded := strings.Split(value, ".")
	if len(encoded) != 2 {
		return nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded[0])
	if err != nil {
		return nil
	}
	signature, err := base64.RawURLEncoding.DecodeString(encoded[1])
	if err != nil || !validAcquisitionMAC(h.acquisitionKey, data, signature) {
		return nil
	}
	var payload acquisitionCookiePayload
	if json.Unmarshal(data, &payload) != nil || payload.OwnerID != ownerID || payload.SessionHash != sessionHash {
		return nil
	}
	if len(payload.Entries) > maxAcquisitionEntries {
		payload.Entries = payload.Entries[len(payload.Entries)-maxAcquisitionEntries:]
	}
	return payload.Entries
}

func (h *Handler) rememberAcquisition(w http.ResponseWriter, r *http.Request, connection, language string, entry opds.Entry, href, sourceID string) {
	if sourceID == "" || connection == "" || entry.ID == "" {
		return
	}
	ownerID, sessionHash, ok := acquisitionBinding(r)
	if !ok {
		return
	}
	entries := h.acquisitionState(r)
	updated := acquisitionEntryState{Connection: connection, Language: language, EntryID: entry.ID, HrefDigest: acquisitionHrefDigest(href), SourceID: sourceID}
	filtered := make([]acquisitionEntryState, 0, len(entries)+1)
	for _, item := range entries {
		if item.Connection == updated.Connection && item.Language == updated.Language && item.EntryID == updated.EntryID && ((item.HrefDigest != "" && item.HrefDigest == updated.HrefDigest) || (item.HrefDigest == "" && item.Href == updated.Href)) {
			continue
		}
		filtered = append(filtered, item)
	}
	filtered = append(filtered, updated)
	if len(filtered) > maxAcquisitionEntries {
		filtered = filtered[len(filtered)-maxAcquisitionEntries:]
	}
	payload := acquisitionCookiePayload{OwnerID: ownerID, SessionHash: sessionHash, Entries: filtered}
	value, boundedEntries := boundedAcquisitionCookie(h.acquisitionKey, payload)
	if value == "" || len(boundedEntries) == 0 {
		h.clearAcquisition(w)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: acquisitionCookie, Value: value, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(h.services.SessionLifetime.Seconds())})
}

func acquisitionHrefDigest(href string) string {
	sum := sha256.Sum256([]byte(href))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func acquisitionBinding(r *http.Request) (string, string, bool) {
	u, ok := webauth.UserFromContext(r.Context())
	if !ok || u.ID == "" {
		return "", "", false
	}
	session, err := r.Cookie(webauth.CookieName)
	if err != nil || session.Value == "" {
		return "", "", false
	}
	return u.ID, auth.HashSessionToken(session.Value), true
}

func encodeAcquisitionCookie(key []byte, payload acquisitionCookiePayload) ([]byte, string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	value := base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return data, value, nil
}

func boundedAcquisitionCookie(key []byte, payload acquisitionCookiePayload) (string, []acquisitionEntryState) {
	entries := payload.Entries
	for len(entries) > 0 {
		payload.Entries = entries
		_, value, err := encodeAcquisitionCookie(key, payload)
		if err == nil && len(value) <= maxAcquisitionCookieBytes {
			return value, entries
		}
		entries = entries[1:]
	}
	return "", nil
}

func validAcquisitionMAC(key, data, signature []byte) bool {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return hmac.Equal(mac.Sum(nil), signature)
}

func (h *Handler) clearAcquisition(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: acquisitionCookie, Path: "/", HttpOnly: true, Secure: h.services.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

type acquisitionTarget struct {
	Connection string     `json:"connection"`
	Language   string     `json:"language"`
	Entry      opds.Entry `json:"entry"`
	Href       string     `json:"href"`
}

const acquisitionTargetTokenPrefix = "m1."

func encodeAcquisitionTarget(key []byte, target acquisitionTarget) string {
	data, err := json.Marshal(target)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return ""
	}
	digest := acquisitionHrefDigest(target.Href)
	return acquisitionTargetTokenPrefix + digest + "." + base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, data, nil))
}

func decodeAcquisitionTarget(key []byte, token string) (acquisitionTarget, error) {
	if !strings.HasPrefix(token, acquisitionTargetTokenPrefix) {
		return acquisitionTarget{}, errors.New("invalid acquisition token")
	}
	parts := strings.SplitN(strings.TrimPrefix(token, acquisitionTargetTokenPrefix), ".", 2)
	if len(parts) != 2 || parts[0] == "" {
		return acquisitionTarget{}, errors.New("invalid acquisition token")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return acquisitionTarget{}, errors.New("invalid acquisition token")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return acquisitionTarget{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return acquisitionTarget{}, errors.New("invalid acquisition token")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return acquisitionTarget{}, errors.New("invalid acquisition token")
	}
	var target acquisitionTarget
	if err := json.Unmarshal(plain, &target); err != nil || target.Connection == "" || target.Href == "" || acquisitionHrefDigest(target.Href) != parts[0] {
		return acquisitionTarget{}, errors.New("invalid acquisition token")
	}
	return target, nil
}

func isAcquisitionTargetToken(value string) bool {
	return strings.HasPrefix(value, acquisitionTargetTokenPrefix)
}

func (h *Handler) clientTargetToken(connection, language string, entry *opds.Entry, href string) string {
	if isAcquisitionTargetToken(href) {
		return href
	}
	target := acquisitionTarget{Connection: connection, Language: language, Href: href}
	if entry != nil {
		target.Entry = opds.Entry{ID: entry.ID, Title: entry.Title}
	}
	return encodeAcquisitionTarget(h.targetKey, target)
}

func (h *Handler) acquisitionEntryForClient(connection, language string, entry opds.Entry, href string) (opds.Entry, string) {
	token := h.clientTargetToken(connection, language, &entry, href)
	entry.Links = []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: token}}
	return entry, token
}

func (h *Handler) acquisitionReturnPath(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "/library"
	}
	return webauth.SafeReturnPath(raw)
}

func bookIDFromReturnPath(raw string) string {
	parsed, err := url.Parse(webauth.SafeReturnPath(raw))
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return ""
	}
	if bookID := strings.TrimSpace(parsed.Query().Get("book_id")); bookID != "" {
		return bookID
	}
	const booksPrefix = "/books/"
	if strings.HasPrefix(parsed.Path, booksPrefix) {
		bookID := strings.TrimPrefix(parsed.Path, booksPrefix)
		if bookID != "" && !strings.Contains(bookID, "/") {
			if decoded, decodeErr := url.PathUnescape(bookID); decodeErr == nil {
				return strings.TrimSpace(decoded)
			}
		}
	}
	return ""
}

func safeAcquisitionURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveURLParameter(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func connectionURLForEdit(connection domain.OpdsConnection) string {
	if connectionURLContainsCredentials(connection.URL) {
		return ""
	}
	return connection.URL
}

func connectionURLContainsCredentials(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	if parsed.User != nil {
		return true
	}
	for key := range parsed.Query() {
		if sensitiveURLParameter(key) {
			return true
		}
	}
	return false
}

func sensitiveURLParameter(key string) bool {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(key)), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' '
	})
	for _, part := range parts {
		switch part {
		case "pass", "password", "passwd", "pwd", "passcode", "secret", "token", "auth", "authorization", "credential", "credentials", "session", "cookie", "signature", "sig", "key", "apikey", "accesskey":
			return true
		}
	}
	return false
}

func addQueryMessage(raw, message string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := u.Query()
	query.Set("message", message)
	u.RawQuery = query.Encode()
	return u.RequestURI()
}
