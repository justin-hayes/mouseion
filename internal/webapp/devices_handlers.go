package webapp

import (
	"errors"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const deviceNameError = "Enter a Device name of 1 to 60 characters."

// devices lists the learner's Devices with the form to add one.
func (h *Handler) devices(w http.ResponseWriter, r *http.Request) {
	h.renderDevices(w, r, http.StatusOK, DevicesView{})
}

// addDevice issues a Device credential and renders it in the response itself:
// there is no redirect, so the password never reaches a URL or a later load.
func (h *Handler) addDevice(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	name := r.FormValue("name")
	credential, err := h.services.Store.Devices.IssueDevice(r.Context(), user(r).ID, name)
	if errors.Is(err, auth.ErrInvalidDeviceName) {
		h.renderDevices(w, r, http.StatusUnprocessableEntity, DevicesView{ErrorMessage: deviceNameError, Name: name})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	h.renderDevices(w, r, http.StatusOK, DevicesView{Issued: &credential, KosyncAddress: "http://" + r.Host + "/kosync"})
}

func (h *Handler) renderDevices(w http.ResponseWriter, r *http.Request, status int, view DevicesView) {
	u := user(r)
	devices, err := h.services.Store.Devices.ListDevices(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	view.Devices = devices
	w.Header().Set("Cache-Control", "no-store")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	render(w, r, DevicesPage(u, h.csrf(w, r), view))
}

func deviceLastSeen(device domain.Device) string {
	if device.LastSeenAt == nil || device.LastSeenAt.IsZero() {
		return "Never"
	}
	return completedAtLabel(device.LastSeenAt)
}
