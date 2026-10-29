package chat

import (
	"errors"
	"testing"
	"time"
)

func TestPagerWantsOnlyNearTheTop(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var p Pager
	if p.Want(olderThreshold, 100, now) {
		t.Fatal("asked while scrolled away from the top")
	}
	if p.Want(0, 0, now) {
		t.Fatal("asked with nothing shown")
	}
	if !p.Want(-1, 100, now) {
		t.Fatal("did not ask with the header row visible")
	}
	if p.Want(0, 100, now) {
		t.Fatal("asked again while a page is in flight")
	}
}

func TestPagerStopsAtTheStart(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var p Pager
	p.Want(0, 10, now)
	p.Done(30, nil, now)
	if p.Loading || p.AtStart {
		t.Fatalf("after a full page: %+v", p)
	}
	if !p.Want(0, 40, now) {
		t.Fatal("did not ask for the next page")
	}
	p.Done(0, nil, now)
	if !p.AtStart {
		t.Fatal("an empty page did not mark the start")
	}
	if p.Want(0, 40, now) {
		t.Fatal("asked past the start")
	}
}

func TestPagerWaitsAfterAFailure(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var p Pager
	p.Want(0, 10, now)
	p.Done(0, errors.New("network down"), now)
	if p.AtStart {
		t.Fatal("a failure marked the start")
	}
	if p.Want(0, 10, now.Add(pagerRetry-time.Second)) {
		t.Fatal("asked again before the retry wait")
	}
	if !p.Want(0, 10, now.Add(pagerRetry)) {
		t.Fatal("did not ask again after the retry wait")
	}
}
