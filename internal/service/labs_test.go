package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func TestListTodayReturnsOnlyLabsOverlappingToday_AC3(t *testing.T) {
	st := servicetest.NewStore()
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	st.Labs = []store.Lab{
		{ID: 1, Title: "Today", StartsAt: day.Add(9 * time.Hour), EndsAt: day.Add(12 * time.Hour)},
		{ID: 2, Title: "Since yesterday", StartsAt: day.Add(-2 * time.Hour), EndsAt: day.Add(time.Hour)},
		{ID: 3, Title: "Tomorrow", StartsAt: day.Add(33 * time.Hour), EndsAt: day.Add(36 * time.Hour)},
		{ID: 4, Title: "Yesterday", StartsAt: day.Add(-5 * time.Hour), EndsAt: day},
	}
	l := service.NewLabs(st, clock, time.UTC)
	labs, err := l.ListToday(context.Background(), service.Caller{ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 2 || labs[0].ID != 1 || labs[1].ID != 2 {
		t.Fatalf("labs = %+v, want ids 1 and 2", labs)
	}
}

func TestListTodayNeedsASignedInCaller_AC9(t *testing.T) {
	l := service.NewLabs(servicetest.NewStore(), clock, time.UTC)
	if _, err := l.ListToday(context.Background(), service.Caller{}); err != service.ErrUnauthenticated {
		t.Fatalf("err = %v", err)
	}
}

func TestListTodayStoreError(t *testing.T) {
	st := servicetest.NewStore()
	st.ErrLabs = errDB
	l := service.NewLabs(st, clock, time.UTC)
	if _, err := l.ListToday(context.Background(), service.Caller{ID: 7}); !errors.Is(err, errDB) {
		t.Fatalf("err = %v", err)
	}
}
