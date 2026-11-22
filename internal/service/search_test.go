package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// fakeModel returns out (or err) and records what it was asked. With block set it waits for the
// deadline, as a model that never answers.
type fakeModel struct {
	mu    sync.Mutex
	out   string
	err   error
	block bool
	calls int
	query string
	today time.Time
}

func (m *fakeModel) SearchFilters(ctx context.Context, query string, today time.Time, _ *time.Location) (json.RawMessage, error) {
	m.mu.Lock()
	m.calls, m.query, m.today = m.calls+1, query, today
	m.mu.Unlock()
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return json.RawMessage(m.out), m.err
}

// searchStore has, with the clock at 10:00 on Thu 22 Oct:
//   - 1 Optics (4 places, "Lenses and lasers"): today 08:00–09:00 (over), today 13:00–15:00 (afternoon),
//     tomorrow 18:00–20:00 (evening, 3 booked: 1 free)
//   - 2 Robotics (8 places, "Line-following robots"): tomorrow 09:00–11:00 (morning), in 10 days 09:00
func searchStore(t *testing.T) *servicetest.Store {
	t.Helper()
	st := servicetest.NewStore()
	st.Labs = []store.Lab{
		{ID: 1, Title: "Optics", Description: "Lenses and lasers", PlaceLimit: 4, OwnerID: staff.ID},
		{ID: 2, Title: "Robotics", Description: "Line-following robots", PlaceLimit: 8, OwnerID: staff.ID},
	}
	st.LabSessions = []servicetest.LabSession{
		sess(1, 1, 8, 9), sess(2, 1, 13, 15), sess(3, 1, 42, 44), sess(4, 2, 33, 35), sess(5, 2, 249, 251),
	}
	for _, u := range []int64{20, 21, 22} {
		if _, err := st.BookSession(context.Background(), u, 3, func(store.SessionView, bool) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func searchLabs(t *testing.T, out string) (*service.Labs, *fakeModel, *servicetest.Store) {
	t.Helper()
	st := searchStore(t)
	m := &fakeModel{out: out}
	return service.NewLabs(st, clock, time.UTC).WithSearchModel(m), m, st
}

func sessionIDs(cs []service.SessionCard) []int64 {
	var ids []int64
	for _, c := range cs {
		ids = append(ids, c.SessionID)
	}
	return ids
}

func date(s string) time.Time {
	d, err := time.ParseInLocation(service.DateLayout, s, time.UTC)
	if err != nil {
		panic(err)
	}
	return d
}

func TestSmartSearchLabsTomorrowListsOnlyTomorrow_AC31(t *testing.T) {
	l, m, _ := searchLabs(t, `{"date_from": "2026-10-23", "date_to": "2026-10-23"}`)
	f, err := l.SmartSearch(context.Background(), student, "  labs tomorrow ")
	if err != nil {
		t.Fatal(err)
	}
	if !f.From.Equal(date("2026-10-23")) || !f.To.Equal(date("2026-10-23")) {
		t.Fatalf("filters = %+v, want tomorrow only", f)
	}
	if m.query != "labs tomorrow" || !m.today.Equal(fixedNow) {
		t.Errorf("the model got %q at %v", m.query, m.today)
	}
	list, err := l.ListFiltered(context.Background(), student, f)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionIDs(list.Sessions); !slices.Equal(got, []int64{4, 3}) {
		t.Fatalf("sessions = %v, want tomorrow's 4 then 3", got)
	}
	if got := f.Values().Encode(); got != "from=2026-10-23&to=2026-10-23" {
		t.Errorf("chips = %s", got)
	}
}

func TestSmartSearchDiscardsOutputThatIsNotTheFilterObject_AC32(t *testing.T) {
	for name, out := range map[string]string{
		"not JSON":            `date_from: tomorrow`,
		"cut off":             `{"date_from": "2026-10-23"`,
		"two objects":         `{} {}`,
		"an array":            `[]`,
		"unknown field":       `{"date_from": "2026-10-23", "lab_id": 4}`,
		"text min places":     `{"min_free_places": "five"}`,
		"fractional places":   `{"min_free_places": 2.5}`,
		"zero places":         `{"min_free_places": 0}`,
		"past range":          `{"date_from": "2026-10-01", "date_to": "2026-10-21"}`,
		"reversed range":      `{"date_from": "2026-10-25", "date_to": "2026-10-23"}`,
		"too long a range":    `{"date_from": "2026-10-22", "date_to": "2026-12-01"}`,
		"not a date":          `{"date_from": "tomorrow"}`,
		"other time of day":   `{"time_of_day": "night"}`,
		"too many keywords":   `{"keywords": ["a", "b", "c", "d", "e", "f"]}`,
		"keywords not a list": `{"keywords": "quiet"}`,
	} {
		l, _, _ := searchLabs(t, out)
		if _, err := l.SmartSearch(context.Background(), student, "labs"); !errors.Is(err, service.ErrSearchUnavailable) {
			t.Errorf("%s: err = %v, want ErrSearchUnavailable", name, err)
		}
	}
}

func TestSmartSearchRefusesLongQueriesBeforeAnyCall_AC33(t *testing.T) {
	l, m, _ := searchLabs(t, `{}`)
	if _, err := l.SmartSearch(context.Background(), student, strings.Repeat("é", 201)); err != service.ErrSearchTooLong {
		t.Fatalf("201 characters: err = %v, want ErrSearchTooLong", err)
	}
	if _, err := l.SmartSearch(context.Background(), student, "   "); err != service.ErrSearchEmpty {
		t.Fatalf("blank: err = %v, want ErrSearchEmpty", err)
	}
	if m.calls != 0 {
		t.Fatalf("the model was called %d times", m.calls)
	}
	// Exactly 200 characters is allowed; they are counted as characters, not bytes.
	if _, err := l.SmartSearch(context.Background(), student, strings.Repeat("é", 200)); err != nil || m.calls != 1 {
		t.Fatalf("200 characters: err = %v, calls %d", err, m.calls)
	}
	if got := service.ErrSearchTooLong.Error(); got != "Please keep your search to 200 characters or fewer." {
		t.Errorf("message = %q", got)
	}
}

func TestSmartSearchAllowsTenSearchesAMinute_AC58(t *testing.T) {
	now := fixedNow
	m := &fakeModel{out: `{}`}
	l := service.NewLabs(searchStore(t), func() time.Time { return now }, time.UTC).WithSearchModel(m)
	for i := range 10 {
		if _, err := l.SmartSearch(context.Background(), student, "labs"); err != nil {
			t.Fatalf("search %d: %v", i+1, err)
		}
		now = now.Add(5 * time.Second)
	}
	if _, err := l.SmartSearch(context.Background(), student, "labs"); !errors.Is(err, service.ErrSearchUnavailable) || m.calls != 10 {
		t.Fatalf("11th search: err = %v, calls %d; want unavailable with no call", err, m.calls)
	}
	// Another user has their own ten.
	if _, err := l.SmartSearch(context.Background(), staff, "labs"); err != nil {
		t.Fatalf("staff: %v", err)
	}
	// A minute after the first search, one slot is free again.
	now = fixedNow.Add(time.Minute + time.Second)
	if _, err := l.SmartSearch(context.Background(), student, "labs"); err != nil {
		t.Fatalf("a minute later: %v", err)
	}
	if _, err := l.SmartSearch(context.Background(), student, "labs"); !errors.Is(err, service.ErrSearchUnavailable) {
		t.Fatalf("then over the limit again: err = %v", err)
	}
}

func TestSmartSearchAbandonsTheModelAtTheDeadline_AC56(t *testing.T) {
	if service.SearchTimeout != 5*time.Second {
		t.Fatalf("SearchTimeout = %v, want 5s", service.SearchTimeout)
	}
	m := &fakeModel{block: true}
	l := service.NewLabs(searchStore(t), clock, time.UTC).WithSearchModel(m).WithSearchTimeout(30 * time.Millisecond)
	start := time.Now()
	_, err := l.SmartSearch(context.Background(), student, "labs")
	if !errors.Is(err, service.ErrSearchUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want unavailable because of the deadline", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("took %v", took)
	}
}

func TestSmartSearchFailuresAreUnavailable_AC27(t *testing.T) {
	cause := errors.New("429 rate limited")
	l, m, _ := searchLabs(t, "")
	m.err = cause
	_, err := l.SmartSearch(context.Background(), student, "labs")
	if !errors.Is(err, service.ErrSearchUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("err = %v, want unavailable wrapping the cause", err)
	}
	if got := service.ErrSearchUnavailable.Error(); got != "Smart search is unavailable, showing all labs." {
		t.Errorf("message = %q", got)
	}
	off := service.NewLabs(searchStore(t), clock, time.UTC)
	if off.SearchEnabled() {
		t.Error("search is on without a model")
	}
	if _, err := off.SmartSearch(context.Background(), student, "labs"); !errors.Is(err, service.ErrSearchUnavailable) {
		t.Errorf("no model: err = %v", err)
	}
	if _, err := l.SmartSearch(context.Background(), service.Caller{}, "labs"); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v", err)
	}
	if !l.SearchEnabled() {
		t.Error("search is off with a model")
	}
}

func TestSmartSearchFillsInAndClampsDates(t *testing.T) {
	for out, want := range map[string][2]string{
		`{}`:                          {"2026-10-22", "2026-10-28"}, // no date: the next seven days
		`{"date_from": "2026-10-24"}`: {"2026-10-24", "2026-10-24"}, // one day
		`{"date_to": "2026-10-24"}`:   {"2026-10-22", "2026-10-24"}, // from today
		`{"date_from": "2026-10-20", "date_to": "2026-10-23"}`:                           {"2026-10-22", "2026-10-23"}, // started earlier
		`{"keywords": [" optics ", ""], "time_of_day": "evening", "min_free_places": 1}`: {"2026-10-22", "2026-10-28"},
	} {
		l, _, _ := searchLabs(t, out)
		f, err := l.SmartSearch(context.Background(), student, "labs")
		if err != nil {
			t.Fatalf("%s: %v", out, err)
		}
		if got := [2]string{f.From.Format(service.DateLayout), f.To.Format(service.DateLayout)}; got != want {
			t.Errorf("%s: range %v, want %v", out, got, want)
		}
	}
	l, _, _ := searchLabs(t, `{"keywords": [" optics ", ""], "time_of_day": "evening", "min_free_places": 1}`)
	f, _ := l.SmartSearch(context.Background(), student, "labs")
	if !slices.Equal(f.Keywords, []string{"optics"}) || f.TimeOfDay != service.Evening || f.MinFree != 1 {
		t.Fatalf("filters = %+v", f)
	}
}

func TestListFilteredAppliesEachFilter_AC55(t *testing.T) {
	l, _, st := searchLabs(t, `{}`)
	week := service.Filters{From: date("2026-10-22"), To: date("2026-10-28")}
	ctx := context.Background()
	for name, tc := range map[string]struct {
		f    func(service.Filters) service.Filters
		want []int64
	}{
		"the week":  {func(f service.Filters) service.Filters { return f }, []int64{1, 2, 4, 3}},
		"morning":   {func(f service.Filters) service.Filters { f.TimeOfDay = service.Morning; return f }, []int64{1, 4}},
		"afternoon": {func(f service.Filters) service.Filters { f.TimeOfDay = service.Afternoon; return f }, []int64{2}},
		"evening":   {func(f service.Filters) service.Filters { f.TimeOfDay = service.Evening; return f }, []int64{3}},
		// 4 free: today's afternoon Optics and tomorrow's Robotics; the over session cannot be booked.
		"4 free": {func(f service.Filters) service.Filters { f.MinFree = 4; return f }, []int64{2, 4}},
		"1 free": {func(f service.Filters) service.Filters { f.MinFree = 1; return f }, []int64{2, 4, 3}},
		// Any keyword, in the title or the description, ignoring case.
		"keywords": {func(f service.Filters) service.Filters { f.Keywords = []string{"LASERS", "nothing"}; return f }, []int64{1, 2, 3}},
		"no match": {func(f service.Filters) service.Filters { f.Keywords = []string{"chemistry"}; return f }, nil},
		"ten days": {func(f service.Filters) service.Filters {
			f.From, f.To = date("2026-11-01"), date("2026-11-01")
			return f
		}, []int64{5}},
	} {
		list, err := l.ListFiltered(ctx, student, tc.f(week))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := sessionIDs(list.Sessions); !slices.Equal(got, tc.want) {
			t.Errorf("%s: sessions %v, want %v", name, got, tc.want)
		}
	}
	// A cancelled session is shown, but not when free places are asked for.
	if _, err := l.CancelSession(ctx, staff, 4); err != nil {
		t.Fatal(err)
	}
	if list, _ := l.ListFiltered(ctx, student, service.Filters{From: date("2026-10-23"), To: date("2026-10-23"), MinFree: 1}); !slices.Equal(sessionIDs(list.Sessions), []int64{3}) {
		t.Errorf("cancelled with min free: %v", sessionIDs(list.Sessions))
	}
	// The student's own bookings are marked, as on the day list.
	if _, err := l.Book(ctx, student, 2); err != nil {
		t.Fatal(err)
	}
	list, _ := l.ListFiltered(ctx, student, week)
	if !list.Sessions[1].Mine || list.Sessions[0].Mine {
		t.Errorf("mine = %v %v", list.Sessions[0].Mine, list.Sessions[1].Mine)
	}
	if _, err := l.ListFiltered(ctx, service.Caller{}, week); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v", err)
	}
	st.ErrLabs = errDB
	if _, err := l.ListFiltered(ctx, student, week); !errors.Is(err, errDB) {
		t.Errorf("store error: err = %v", err)
	}
	st.ErrLabs, st.ErrSessionsBookedBy = nil, errDB
	if _, err := l.ListFiltered(ctx, student, week); !errors.Is(err, errDB) {
		t.Errorf("marking error: err = %v", err)
	}
}

func TestParseFiltersReadsTheChips_AC55(t *testing.T) {
	l, _, _ := searchLabs(t, `{}`)
	f, err := l.ParseFilters(url.Values{"from": {"2026-10-23"}, "to": {"2026-10-25"}, "time": {"morning"}, "min": {" 3 "}, "kw": {"optics, lasers ,"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Values().Encode(); got != "from=2026-10-23&kw=optics%2C+lasers&min=3&time=morning&to=2026-10-25" {
		t.Fatalf("round trip = %s", got)
	}
	// Removing a chip widens the filters: no dates is the next seven days, no end is the start's day.
	for v, want := range map[string]string{
		"":                              "from=2026-10-22&to=2026-10-28",
		"from=2026-10-24":               "from=2026-10-24&to=2026-10-24",
		"to=2026-10-24":                 "from=2026-10-22&to=2026-10-24",
		"from=2026-10-01&to=2026-10-02": "from=2026-10-01&to=2026-10-02", // past days may be looked at, as on the day list
	} {
		q, _ := url.ParseQuery(v)
		f, err := l.ParseFilters(q)
		if err != nil || f.Values().Encode() != want {
			t.Errorf("%q: %s, %v; want %s", v, f.Values().Encode(), err, want)
		}
	}
	for v, want := range map[string]error{
		"from=23.10.2026":               service.ErrInvalidDate,
		"to=soon":                       service.ErrInvalidDate,
		"from=2026-10-25&to=2026-10-23": service.ErrSearchRangeOrder,
		"from=2026-10-01&to=2026-11-01": service.ErrSearchRangeTooLong,
		"time=night":                    service.ErrSearchTimeOfDay,
		"min=0":                         service.ErrSearchMinFree,
		"min=two":                       service.ErrSearchMinFree,
		"kw=a,b,c,d,e,f":                service.ErrSearchKeywords,
		"kw=" + strings.Repeat("x", 51): service.ErrSearchKeywords,
	} {
		q, _ := url.ParseQuery(v)
		if _, err := l.ParseFilters(q); err != want {
			t.Errorf("%q: err = %v, want %v", v, err, want)
		}
	}
	// 31 days is the longest range.
	if _, err := l.ParseFilters(url.Values{"from": {"2026-10-01"}, "to": {"2026-10-31"}}); err != nil {
		t.Errorf("31 days: %v", err)
	}
}
