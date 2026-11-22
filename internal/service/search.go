package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Smart search (Slice 7): a student's free-text query becomes filters for the normal session list.
const (
	// SearchTimeout is how long a search may wait for the model, retries included (AC-56).
	SearchTimeout = 5 * time.Second
	// MaxQueryLen is the longest query sent to the model, in characters (AC-33).
	MaxQueryLen = 200
	// searchesPerMinute is how many searches one user may make in a sliding minute (AC-58).
	searchesPerMinute = 10
	// maxRangeDays is the longest date range a filter may cover, so one search stays one small query.
	maxRangeDays = 31
	// defaultRangeDays is the range when no date is given: today and the six days after it.
	defaultRangeDays = 7
	maxKeywords      = 5
	maxKeywordLen    = 50
)

// Times of day, by when a session starts in the server's timezone.
const (
	Morning   = "morning"   // before 12:00
	Afternoon = "afternoon" // 12:00 to 17:00
	Evening   = "evening"   // from 17:00
)

// SearchModel turns a query into a filter object, as JSON. internal/model holds the real one; it is
// given only the query, today's date and the timezone, never user or booking data. Its output is not
// trusted: SmartSearch validates every field.
type SearchModel interface {
	SearchFilters(ctx context.Context, query string, today time.Time, loc *time.Location) (json.RawMessage, error)
}

// WithSearchModel turns smart search on. Without it, searches are unavailable and the page hides the box.
func (l *Labs) WithSearchModel(m SearchModel) *Labs {
	l.search = m
	if l.searchTimeout == 0 {
		l.searchTimeout = SearchTimeout
	}
	return l
}

// WithSearchTimeout shortens the model's deadline, for tests that check it is enforced.
func (l *Labs) WithSearchTimeout(d time.Duration) *Labs {
	l.searchTimeout = d
	return l
}

// SearchEnabled tells the page whether to show the search box.
func (l *Labs) SearchEnabled() bool { return l.search != nil }

// Filters are what a search applies to the session list, shown as editable chips. From and To are
// midnights in the server's timezone, both days included.
type Filters struct {
	From, To  time.Time
	TimeOfDay string
	// MinFree is the fewest free places a session must have; 0 means any.
	MinFree  int
	Keywords []string
}

// Values are the filters as URL parameters, the chips' form fields: from, to, time, min and kw.
func (f Filters) Values() url.Values {
	v := url.Values{"from": {f.From.Format(DateLayout)}, "to": {f.To.Format(DateLayout)}}
	if f.TimeOfDay != "" {
		v.Set("time", f.TimeOfDay)
	}
	if f.MinFree > 0 {
		v.Set("min", strconv.Itoa(f.MinFree))
	}
	if len(f.Keywords) > 0 {
		v.Set("kw", strings.Join(f.Keywords, ", "))
	}
	return v
}

// FilterParams are the URL parameters ParseFilters reads; any of them on /labs means a filtered list.
var FilterParams = []string{"from", "to", "time", "min", "kw"}

// ParseFilters reads the chips from a URL, as typed or edited by hand. A missing start is today, a
// missing end is the start's day, and with neither the next seven days are shown.
func (l *Labs) ParseFilters(v url.Values) (Filters, error) {
	var min int
	if s := strings.TrimSpace(v.Get("min")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return Filters{}, ErrSearchMinFree
		}
		min = n
	}
	return l.filters(strings.TrimSpace(v.Get("from")), strings.TrimSpace(v.Get("to")),
		strings.TrimSpace(v.Get("time")), min, strings.Split(v.Get("kw"), ","))
}

// filters checks and completes one set of filters, from the chips or from the model. Callers have
// already refused a min below 1, so 0 here means any.
func (l *Labs) filters(from, to, timeOfDay string, min int, keywords []string) (Filters, error) {
	today := l.midnight(l.now())
	f := Filters{TimeOfDay: timeOfDay, MinFree: min}
	var err error
	switch {
	case from == "" && to == "":
		f.From, f.To = today, today.AddDate(0, 0, defaultRangeDays-1)
	default:
		f.From = today
		if from != "" {
			if f.From, err = time.ParseInLocation(DateLayout, from, l.loc); err != nil {
				return Filters{}, ErrInvalidDate
			}
		}
		f.To = f.From
		if to != "" {
			if f.To, err = time.ParseInLocation(DateLayout, to, l.loc); err != nil {
				return Filters{}, ErrInvalidDate
			}
		}
	}
	switch {
	case f.To.Before(f.From):
		return Filters{}, ErrSearchRangeOrder
	case f.From.AddDate(0, 0, maxRangeDays).Before(f.To.AddDate(0, 0, 1)):
		return Filters{}, ErrSearchRangeTooLong
	case !slices.Contains([]string{"", Morning, Afternoon, Evening}, timeOfDay):
		return Filters{}, ErrSearchTimeOfDay
	}
	for _, k := range keywords {
		if k = strings.TrimSpace(k); k != "" {
			f.Keywords = append(f.Keywords, k)
		}
		if utf8.RuneCountInString(k) > maxKeywordLen {
			return Filters{}, ErrSearchKeywords
		}
	}
	if len(f.Keywords) > maxKeywords {
		return Filters{}, ErrSearchKeywords
	}
	return f, nil
}

// modelOutput is the filter object the model must return: exactly these fields, all optional.
type modelOutput struct {
	DateFrom      string   `json:"date_from"`
	DateTo        string   `json:"date_to"`
	TimeOfDay     string   `json:"time_of_day"`
	MinFreePlaces *int     `json:"min_free_places"`
	Keywords      []string `json:"keywords"`
}

// SmartSearch asks the model for filters (Slice 7). A query that is empty or over 200 characters is
// refused before any call (AC-33). Every failure of the model is ErrSearchUnavailable, wrapping the
// cause for the log: no model, too many searches (AC-58), the 5 second deadline (AC-56), an API
// error (AC-27) or output that is not exactly the filter object (AC-32). Nothing is written.
func (l *Labs) SmartSearch(ctx context.Context, caller Caller, query string) (Filters, error) {
	if caller.ID == 0 {
		return Filters{}, ErrUnauthenticated
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return Filters{}, ErrSearchEmpty
	}
	if utf8.RuneCountInString(query) > MaxQueryLen {
		return Filters{}, ErrSearchTooLong
	}
	if l.search == nil {
		return Filters{}, unavailable(errors.New("no search model configured"))
	}
	if !l.limiter.allow(caller.ID, l.now()) {
		return Filters{}, unavailable(errors.New("rate limit: too many searches in a minute"))
	}
	ctx, cancel := context.WithTimeout(ctx, l.searchTimeout)
	defer cancel()
	raw, err := l.search.SearchFilters(ctx, query, l.now(), l.loc)
	if err != nil {
		return Filters{}, unavailable(err)
	}
	f, err := l.modelFilters(raw)
	if err != nil {
		return Filters{}, unavailable(err)
	}
	return f, nil
}

func unavailable(cause error) error {
	return fmt.Errorf("%w (%w)", ErrSearchUnavailable, cause)
}

// modelFilters decodes the model's output strictly: one JSON object, no unknown fields, whole numbers
// only. A range that ends before today is discarded; one that starts before today starts today.
func (l *Labs) modelFilters(raw json.RawMessage) (Filters, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var out modelOutput
	if err := dec.Decode(&out); err != nil {
		return Filters{}, fmt.Errorf("model output: %w", err)
	}
	if dec.More() {
		return Filters{}, errors.New("model output: more than one value")
	}
	min := 0
	if out.MinFreePlaces != nil {
		if *out.MinFreePlaces < 1 {
			return Filters{}, errors.New("model output: min_free_places below 1")
		}
		min = *out.MinFreePlaces
	}
	if len(out.Keywords) > maxKeywords {
		return Filters{}, errors.New("model output: too many keywords")
	}
	f, err := l.filters(out.DateFrom, out.DateTo, out.TimeOfDay, min, out.Keywords)
	if err != nil {
		return Filters{}, fmt.Errorf("model output: %w", err)
	}
	today := l.midnight(l.now())
	if f.To.Before(today) {
		return Filters{}, errors.New("model output: a date range in the past")
	}
	if f.From.Before(today) {
		f.From = today
	}
	return f, nil
}

// FilteredList is the session list with search filters applied.
type FilteredList struct {
	Filters  Filters
	Sessions []SessionCard
}

// ListFiltered lists the sessions that match the filters, with the same query as the day list (AC-31,
// AC-55, AC-59). A session matches when it starts at the chosen time of day, has at least MinFree
// free places and can still be booked (when MinFree is set), and has any keyword in its title or
// description. It only reads.
func (l *Labs) ListFiltered(ctx context.Context, caller Caller, f Filters) (FilteredList, error) {
	if caller.ID == 0 {
		return FilteredList{}, ErrUnauthenticated
	}
	views, err := l.store.SessionsBetween(ctx, f.From, f.To.AddDate(0, 0, 1))
	if err != nil {
		return FilteredList{}, fmt.Errorf("list sessions: %w", err)
	}
	var cards []SessionCard
	for _, c := range l.cards(views) {
		if l.matches(c, f) {
			cards = append(cards, c)
		}
	}
	if cards, err = l.markMine(ctx, caller, cards); err != nil {
		return FilteredList{}, err
	}
	return FilteredList{Filters: f, Sessions: cards}, nil
}

func (l *Labs) matches(c SessionCard, f Filters) bool {
	if f.TimeOfDay != "" && timeOfDay(c.StartsAt.In(l.loc)) != f.TimeOfDay {
		return false
	}
	if f.MinFree > 0 && (c.FreePlaces < f.MinFree || c.Started || c.Cancelled) {
		return false
	}
	if len(f.Keywords) == 0 {
		return true
	}
	text := strings.ToLower(c.Title + " " + c.Description)
	return slices.ContainsFunc(f.Keywords, func(k string) bool { return strings.Contains(text, strings.ToLower(k)) })
}

func timeOfDay(t time.Time) string {
	switch {
	case t.Hour() < 12:
		return Morning
	case t.Hour() < 17:
		return Afternoon
	}
	return Evening
}

// rateLimiter allows each user a number of searches in a sliding minute. It lives in the process:
// a restart forgets it, and each server instance counts on its own.
type rateLimiter struct {
	mu   sync.Mutex
	hits map[int64][]time.Time
}

func (r *rateLimiter) allow(userID int64, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hits == nil {
		r.hits = map[int64][]time.Time{}
	}
	recent := slices.DeleteFunc(r.hits[userID], func(t time.Time) bool { return !t.After(now.Add(-time.Minute)) })
	if len(recent) >= searchesPerMinute {
		r.hits[userID] = recent
		return false
	}
	r.hits[userID] = append(recent, now)
	return true
}
