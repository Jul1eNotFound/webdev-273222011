package handler_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

const msgInvalidDate = "Please choose a valid date."

// In newFakeApp, Chemistry basics and Circuits workshop are today (Thursday 22 October 2026) and Robotics
// intro is tomorrow. The clock says 08:30.

func TestAC80_LabListShowsTheChosenDay(t *testing.T) {
	base, _ := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs?date=2026-10-23", nil)
	for _, want := range []string{
		"<h1>Labs on Friday 23 October 2026</h1>", "Robotics intro",
		`<a href="/labs?date=2026-10-22">← Previous day</a>`, `<a href="/labs">Today</a>`, `<a href="/labs?date=2026-10-24">Next day →</a>`,
		`<label for="date">Show another day</label>`, `<input id="date" name="date" type="date" value="2026-10-23">`,
		`<form method="get" action="/labs">`, `action="/sessions/3/book"`,
	} {
		wantStatus(t, r, http.StatusOK, want)
	}
	for _, other := range []string{"Chemistry basics", "Circuits workshop"} {
		if strings.Contains(r.body, other) {
			t.Errorf("tomorrow's list shows today's %q", other)
		}
	}
}

func TestAC80_TodayKeepsItsHeadingWithOrWithoutADate(t *testing.T) {
	base, _ := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	for _, path := range []string{"/labs", "/labs?date=2026-10-22", "/labs?date="} {
		r := do(t, c, http.MethodGet, base+path, nil)
		wantStatus(t, r, http.StatusOK, "<h1>Labs available today</h1>")
		wantStatus(t, r, http.StatusOK, `value="2026-10-22"`)
		if strings.Contains(r.body, `<a href="/labs">Today</a>`) {
			t.Errorf("%s links to today while showing today", path)
		}
		if strings.Contains(r.body, "Robotics intro") {
			t.Errorf("%s shows tomorrow's session (AC-22)", path)
		}
	}
}

func TestAC81_InvalidDateIs400WithASentence(t *testing.T) {
	base, _ := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	leak := regexp.MustCompile(`parsing time|cannot parse|SQLSTATE|goroutine`)
	for _, date := range []string{"2026-02-30", "2026-13-01", "22.10.2026", "tomorrow", "2026-10-2"} {
		r := do(t, c, http.MethodGet, base+"/labs?date="+date, nil)
		wantStatus(t, r, http.StatusBadRequest, msgInvalidDate)
		if m := leak.FindString(r.body); m != "" {
			t.Errorf("%s: the page leaks %q", date, m)
		}
	}
}

func TestAC82_AnEmptyDayThatIsNotTodaySaysSo(t *testing.T) {
	base, _ := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs?date=2026-10-30", nil)
	wantStatus(t, r, http.StatusOK, "No labs on this day")
	if strings.Contains(r.body, "No labs available today") || strings.Contains(r.body, "lab-list__item") {
		t.Errorf("the empty day shows the wrong empty state or a lab\n%s", r.body)
	}
}

func TestAC80_DayPageMeetsTheDefinitionOfDone_AC30_AC41(t *testing.T) {
	base, _ := newFakeApp(t)
	r := do(t, signedIn(t, base, studentEmail, studentPass), http.MethodGet, base+"/labs?date=2026-10-23", nil)
	if m := regexp.MustCompile(`(?i)transition|animation|@keyframes`).FindString(r.body); m != "" {
		t.Errorf("the page contains %q", m)
	}
	for _, m := range regexp.MustCompile(`<input id="([^"]+)"`).FindAllStringSubmatch(r.body, -1) {
		if !strings.Contains(r.body, `<label for="`+m[1]+`">`) {
			t.Errorf("input %q has no label", m[1])
		}
	}
}
