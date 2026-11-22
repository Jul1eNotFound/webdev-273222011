package handler

import (
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

// filterView is the chips form: each filter as typed, and a link that removes it.
type filterView struct {
	From, To, Time, Min, Kw string
	// Remove maps a filter (dates, time, min, kw) to the list without it.
	Remove map[string]string
}

func newFilterView(v url.Values) *filterView {
	fv := &filterView{From: v.Get("from"), To: v.Get("to"), Time: v.Get("time"), Min: v.Get("min"), Kw: v.Get("kw"),
		Remove: map[string]string{}}
	without := func(keys ...string) string {
		rest := url.Values{}
		for k, vals := range v {
			if !slices.Contains(keys, k) && slices.Contains(service.FilterParams, k) {
				rest[k] = vals
			}
		}
		if len(rest) == 0 {
			// Every chip removed: the next seven days, still as a search.
			return "/labs?from=&to="
		}
		return "/labs?" + rest.Encode()
	}
	fv.Remove["dates"] = without("from", "to")
	for _, k := range []string{"time", "min", "kw"} {
		if v.Get(k) != "" {
			fv.Remove[k] = without(k)
		}
	}
	return fv
}

// isFilterQuery is true when the URL carries any chip, even an empty one: the list is a search result.
func isFilterQuery(q url.Values) bool {
	return slices.ContainsFunc(service.FilterParams, q.Has)
}

// smartSearch asks the model for filters and sends the browser to the list with them, so the chips are
// in the URL and a reload does not call the model again (AC-31). A query that is too long is refused
// before any call (AC-33). When the model fails, today's unfiltered list is shown with one sentence and
// a 200, never a 500 (AC-27); the cause goes to the log.
func (h *handler) smartSearch(w http.ResponseWriter, r *http.Request, c service.Caller, query string) {
	f, err := h.Labs.SmartSearch(r.Context(), c, query)
	var se *service.Error
	switch {
	case err == nil:
		http.Redirect(w, r, "/labs?"+f.Values().Encode(), http.StatusSeeOther)
	case errors.Is(err, service.ErrSearchUnavailable):
		h.Log.Warn("smart search unavailable", "err", err)
		h.dayPage(w, r, c, "", http.StatusOK, pageData{Notice: service.ErrSearchUnavailable.Message, SearchQuery: query})
	case errors.As(err, &se) && se.Kind == service.KindValidation:
		h.dayPage(w, r, c, "", http.StatusBadRequest, pageData{Error: se.Message, SearchQuery: query})
	default:
		h.fail(w, r, err)
	}
}

// filteredPage lists the sessions that match the chips (AC-55, AC-59). Editing a chip never calls the
// model. A chip that cannot be read is shown again with its sentence and a 400.
func (h *handler) filteredPage(w http.ResponseWriter, r *http.Request, c service.Caller) {
	q := r.URL.Query()
	data := pageData{Title: "Search results · Labwiz", User: &c, SearchEnabled: h.Labs.SearchEnabled()}
	f, err := h.Labs.ParseFilters(q)
	var se *service.Error
	if errors.As(err, &se) {
		data.Error, data.Filtered = se.Message, newFilterView(q)
		h.page(w, r, http.StatusBadRequest, "labs", data)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	list, err := h.Labs.ListFiltered(r.Context(), c, f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// The chips show the filters as applied: a missing date filled in, keywords tidied.
	data.Filtered, data.Sessions = newFilterView(f.Values()), list.Sessions
	h.page(w, r, http.StatusOK, "labs", data)
}

// dayPage is the plain lab list for one day ("" is today), with extra page data such as a message.
func (h *handler) dayPage(w http.ResponseWriter, r *http.Request, c service.Caller, date string, status int, extra pageData) {
	list, err := h.Labs.ListDay(r.Context(), c, date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	extra.Title, extra.User, extra.Sessions, extra.Day = "Labs · Labwiz", &c, list.Sessions, &list
	extra.SearchEnabled = h.Labs.SearchEnabled()
	h.page(w, r, status, "labs", extra)
}

// labsPage is /labs: a smart search (?q=), a filtered list (chips in the URL) or one day's list.
func (h *handler) labsPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	q := r.URL.Query()
	switch {
	case q.Has("q"):
		h.smartSearch(w, r, c, q.Get("q"))
	case isFilterQuery(q):
		h.filteredPage(w, r, c)
	default:
		// No date is today; a date that is not a real day is a 400 through fail (AC-81).
		h.dayPage(w, r, c, q.Get("date"), http.StatusOK, pageData{})
	}
}
