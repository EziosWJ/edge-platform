package edge

import (
	"context"
	"sync"
	"testing"
	"time"
)

type serviceStore struct {
	mu           sync.Mutex
	observations []Observation
	page         Page
}

type registrationServiceStore struct {
	serviceStore
	created bool
}

func (s *registrationServiceStore) ObserveRegistration(_ context.Context, observation Observation) (ObservationResult, error) {
	s.observations = append(s.observations, observation)
	return ObservationResult{
		Edge:    Edge{EdgeID: observation.EdgeID, Status: observation.Status, RegisteredAt: observation.ReceivedAt, LastSeenAt: observation.ReceivedAt},
		Created: s.created,
	}, nil
}

func (s *serviceStore) Observe(_ context.Context, observation Observation) (Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observations = append(s.observations, observation)
	return Edge{EdgeID: observation.EdgeID, Status: observation.Status, RegisteredAt: observation.ReceivedAt, LastSeenAt: observation.ReceivedAt}, nil
}

func (s *serviceStore) Page(_ context.Context, _ PageQuery) (Page, error) { return s.page, nil }

func (s *serviceStore) Detail(_ context.Context, edgeID string) (Edge, error) {
	return Edge{EdgeID: edgeID, Status: StatusOnline}, nil
}

func TestServiceObserveAcceptsOnlyCloudEdgeInputs(t *testing.T) {
	store := &serviceStore{}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 9, 21, 1, 2, 3, 4, time.FixedZone("CST", 8*60*60))
	if _, err := service.Observe(context.Background(), Observation{EdgeID: "edge-01", Status: StatusOnline, ReceivedAt: receivedAt}); err != nil {
		t.Fatal(err)
	}
	if got := store.observations[0].ReceivedAt.Location(); got != time.UTC {
		t.Fatalf("received time location = %v, want UTC", got)
	}
	for _, input := range []Observation{
		{EdgeID: "", Status: StatusOnline, ReceivedAt: receivedAt},
		{EdgeID: "edge-01", Status: Status("UNKNOWN"), ReceivedAt: receivedAt},
		{EdgeID: "edge-01", Status: StatusOffline},
	} {
		if _, err := service.Observe(context.Background(), input); err != ErrInvalid {
			t.Errorf("invalid observation error = %v, want ErrInvalid", err)
		}
	}
}

func TestServicePageRejectsInvalidQueryAndReturnsEmptyRecords(t *testing.T) {
	service, err := NewService(&serviceStore{})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []PageQuery{
		{Page: 0, PageSize: 10},
		{Page: 1, PageSize: 0},
		{Page: 1, PageSize: 501},
		{Page: 1, PageSize: 10, Status: statusPointer(Status("UNKNOWN"))},
	} {
		if _, err := service.Page(context.Background(), query); err != ErrInvalid {
			t.Errorf("Page(%+v) error = %v, want ErrInvalid", query, err)
		}
	}
	page, err := service.Page(context.Background(), PageQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 10 || page.Records == nil {
		t.Fatalf("page = %+v, want empty page", page)
	}
}

func TestServiceNotifiesOnlyAfterTrueEdgeRegistration(t *testing.T) {
	store := &registrationServiceStore{created: true}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	var notifications int
	service.SetRegistrationListener(func() { notifications++ })
	at := time.Date(2026, 9, 21, 1, 2, 3, 0, time.UTC)
	if _, err := service.Observe(context.Background(), Observation{EdgeID: "edge-01", Status: StatusOnline, ReceivedAt: at}); err != nil {
		t.Fatal(err)
	}
	if notifications != 1 {
		t.Fatalf("new Edge notifications = %d, want 1", notifications)
	}

	store.created = false
	if _, err := service.Observe(context.Background(), Observation{EdgeID: "edge-01", Status: StatusOffline, ReceivedAt: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if notifications != 1 {
		t.Fatalf("duplicate Edge notifications = %d, want 1", notifications)
	}
}

func statusPointer(status Status) *Status { return &status }
